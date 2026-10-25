#!/usr/bin/env bash
# Moves the M2 hub behind lodge (docs/plans/0006-m4-notes-inside-lodge.md,
# Phase 7; docs/design/hub.md#moving-devices-to-lodge). Run it on the
# hub's machine as root, once M2's trial has ended:
#
#   scripts/move-hub-behind-lodge.sh                       # dry run: checks and prints the plan
#   scripts/move-hub-behind-lodge.sh --apply --url https://lodge.<tailnet>.ts.net/device/notes
#   scripts/move-hub-behind-lodge.sh --rollback /var/backups/frostnotes-move-<stamp> [--apply] [--force]
#
# What --apply does, in order, printing each command first:
#   1. stops and disables the trial's notesd.service;
#   2. sets aside what frostnotes.service's data directory holds (the
#      overnight test, and the copies of the hub's vaults that lodge's
#      notesd synced as the hub's device, each pushed in full and verified;
#      never merged), then copies the hub's data directory whole into it,
#      owned by frostnotes, and renames the original;
#   3. sets uid= and gid= of the vaults' CIFS mounts in /etc/fstab to
#      frostnotes, and remounts them;
#   4. checks that frostnotes can write to each share (one hidden file at
#      the share's root, written, read back and removed);
#   5. enables and starts frostnotes-lodge.socket and frostnotes.service,
#      and checks that its notesd is the same hub with the same vaults;
#   6. records the URL devices should use (notesctl hub url), and reloads
#      lodge.
#
# It keeps a backup of everything it moves or edits under
# /var/backups/frostnotes-move-<stamp>, with a manifest --rollback reads,
# and never touches the vaults' contents. It refuses any state it cannot
# read unambiguously, before changing anything.
#
# The hub cannot tell devices where it went: step 1 stops it before step 6
# records the URL. So on each device, run `notesctl hub relocate OLD-URL
# NEW-URL`, NEW-URL being the one step 6 records; once `notesctl hub
# devices` shows each device seen, the trial hub's old `tailscale serve`
# can go. When the move set copies of the hub's vaults aside, also revoke
# lodge's former device on the hub with `notesctl hub revoke ID`.
#
# --rollback puts the trial's hub back as it was. Once step 6 has run,
# devices may have followed the hub to lodge and synced there, and the
# trial's data would be behind them (its notesd cannot open the moved
# database, whose schema is newer), so the rollback refuses when the hub's
# log has moved since the move, unless --force.
set -euo pipefail

# Tests stage a root and stub commands (scripts/move_test.go); on a real
# machine all of these are empty or the system's own.
ROOT=${MOVE_ROOT:-}
SYSTEMCTL=${SYSTEMCTL:-systemctl}
RUNUSER=${RUNUSER:-runuser}
GETENT=${GETENT:-getent}
MOUNT=${MOUNT:-mount}
UMOUNT=${UMOUNT:-umount}
CHOWN=${CHOWN:-chown}
SYSTEMD_ESCAPE=${SYSTEMD_ESCAPE:-systemd-escape}
STAMP=${MOVE_STAMP:-$(date -u +%Y%m%dT%H%M%SZ)}

TRIAL_UNIT=notesd.service
UNIT=frostnotes.service
SOCKET_UNIT=frostnotes-lodge.socket
LODGE_UNIT=lodge.service
APP_USER=frostnotes

mode=dry-run
url=
rollback=
force=

usage() {
	sed -n '2,42p' "$0" | sed 's/^# \{0,1\}//'
	exit "${1:-0}"
}

while [ $# -gt 0 ]; do
	case $1 in
	--dry-run) mode=dry-run ;;
	--apply) mode=apply ;;
	--url) url=${2:?--url needs a URL}; shift ;;
	--url=*) url=${1#--url=} ;;
	--rollback) rollback=${2:?--rollback needs a backup directory}; shift ;;
	--force) force=1 ;;
	-h | --help) usage 0 ;;
	*) echo "move-hub-behind-lodge: unknown argument $1" >&2; usage 2 ;;
	esac
	shift
done

p() { printf '%s%s' "$ROOT" "$1"; }
say() { printf '%s\n' "$*"; }
step() { printf '\n== %s\n' "$*"; }
fail() { printf 'move-hub-behind-lodge: %s\n' "$*" >&2; exit 1; }
# run prints a command that changes the machine, and runs it with --apply.
run() {
	printf '  $ %s\n' "$*"
	if [ "$mode" = apply ]; then "$@"; fi
}
# as runs a command as user $1, for reading in either mode.
as() { local u=$1; shift; "$RUNUSER" -u "$u" -- "$@"; }
show() { "$SYSTEMCTL" show -p "$2" --value "$1" 2>/dev/null || true; }
# remount mounts $1 again with /etc/fstab's options: through its systemd
# units when fstab automounts it, else with umount and mount.
remount() {
	local mp=$1 unit
	if awk -v mp="$mp" '!/^[ \t]*#/ && $2 == mp && $4 ~ /x-systemd\.automount/ {f = 1} END {exit !f}' "$(p /etc/fstab)"; then
		unit=$("$SYSTEMD_ESCAPE" -p --suffix=mount "$mp")
		run "$SYSTEMCTL" stop "${unit%.mount}.automount" "$unit"
		run "$SYSTEMCTL" start "${unit%.mount}.automount"
	else
		run "$UMOUNT" "$(p "$mp")" || true
		run "$MOUNT" "$(p "$mp")"
	fi
}
# envof prints variable $2 of unit $1's Environment=, or nothing.
envof() {
	local kv
	for kv in $(show "$1" Environment); do
		case $kv in "$2"=*) printf '%s' "${kv#*=}"; return ;; esac
	done
}
# reload_lodge tells lodge to read its apps again. lodge's own control
# program is not on root's PATH on the hub's machine, so say so when it is
# nowhere to be found rather than run an empty command.
reload_lodge() {
	if [ -n "$LODGECTL" ]; then
		run "$LODGECTL" reload
	else
		say "  ! no lodgectl found: run lodgectl reload yourself"
	fi
}

if [ -z "$ROOT" ] && [ "$(id -u)" -ne 0 ]; then
	fail "run me as root on the hub's machine (sudo $0 …)"
fi

# Where lodge's control program is. lodge.service runs /opt/lodge/bin/lodge
# on the hub's machine, and lodgectl sits beside it there, off root's PATH:
# take the environment's LODGECTL when it is set, then PATH, then that.
lodge_bin=$(show "$LODGE_UNIT" ExecStart | sed -n 's/.*path=\([^ ;]*\).*/\1/p')
LODGECTL=${LODGECTL:-}
if [ -z "$LODGECTL" ]; then
	if lodgectl_found=$(command -v lodgectl); then
		LODGECTL=$lodgectl_found
	elif [ -n "$lodge_bin" ] && [ -x "$(p "$(dirname "$lodge_bin")/lodgectl")" ]; then
		LODGECTL=$(p "$(dirname "$lodge_bin")/lodgectl")
	fi
fi

# ---- rollback -------------------------------------------------------------

if [ -n "$rollback" ]; then
	manifest=$rollback/manifest
	[ -f "$manifest" ] || fail "$manifest is missing: is $rollback a backup of this script?"
	declare -A m
	while IFS='=' read -r k v; do [ -n "$k" ] && m[$k]=$v; done <"$manifest"
	[ -z "${m[rolled_back]:-}" ] || fail "$rollback was rolled back already (${m[rolled_back]})"
	step "Rollback of the move recorded in $rollback ($mode)"
	if [ -n "${m[url_set]:-}" ]; then
		now=$(as "$APP_USER" env FROSTNOTES_SOCKET="${m[app_socket]}" "${m[notesctl]}" hub vaults 2>/dev/null |
			awk -F'\t' '{print $1 ":" $3}' | sort | tr '\n' ' ' || true)
		if [ "$now" = "${m[seqs]:-}" ]; then
			say "  ✓ the hub's log is where the move left it (${m[seqs]})"
		elif [ -z "$force" ]; then
			[ -n "$now" ] || fail "$UNIT does not answer, so whether devices synced through lodge since the move is unknown: start it and run me again, or pass --force"
			fail "the hub's log moved since the move (then ${m[seqs]:-?}, now $now): devices synced through lodge, or the share changed under the hub. Rolling back would leave the trial's hub behind its devices: fix forward, or pass --force if no device synced (notesctl hub devices)"
		else
			say "  ! the hub's log moved since the move (then ${m[seqs]:-?}, now ${now:-unknown}); rolling back anyway (--force)"
		fi
	fi
	if [ "$mode" = apply ]; then
		trap '[ $? -eq 0 ] || printf "move-hub-behind-lodge: the rollback stopped at the command above; what is left is in %s\n" "$rollback" >&2' EXIT
	fi
	run "$SYSTEMCTL" disable --now "$UNIT" "$SOCKET_UNIT"
	if [ -n "${m[target]:-}" ] && [ -e "${m[target]}" ]; then
		run mv "${m[target]}" "$rollback/data-after-move"
	fi
	if [ -n "${m[aside]:-}" ]; then run mv "${m[aside]}" "${m[target]}"; fi
	if [ -n "${m[trial_moved]:-}" ]; then run mv "${m[trial_moved]}" "${m[trial_data]}"; fi
	if [ -f "$rollback/fstab" ]; then
		run cp -p "$rollback/fstab" "$(p /etc/fstab)"
		run "$SYSTEMCTL" daemon-reload
		for mp in ${m[mounts]:-}; do remount "$mp"; done
	fi
	run "$SYSTEMCTL" enable --now "$TRIAL_UNIT"
	if [ "${m[unit_was]:-}" = active ]; then run "$SYSTEMCTL" start "$SOCKET_UNIT" "$UNIT"; fi
	if [ "${m[unit_enabled]:-}" = enabled ]; then run "$SYSTEMCTL" enable "$SOCKET_UNIT" "$UNIT"; fi
	reload_lodge
	if [ "$mode" = apply ]; then printf 'rolled_back=%s\n' "$STAMP" >>"$manifest"; fi
	[ -z "${m[url_set]:-}" ] || say "Devices that followed the hub to ${m[url_set]} stay offline, their notes safe on them, until the move is done again."
	[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply to roll back.'
	exit 0
fi

# ---- what is there --------------------------------------------------------

step "Checks"
problems=()
problem() { problems+=("$*"); say "  ✗ $*"; }
ok() { say "  ✓ $*"; }

if [ -z "$LODGECTL" ]; then
	problem "no lodgectl on PATH or beside $LODGE_UNIT's binary (${lodge_bin:-?}): set LODGECTL=/path/to/lodgectl"
elif [ "$("$SYSTEMCTL" is-active "$LODGE_UNIT" 2>/dev/null)" != active ]; then
	problem "lodge is not up: start $LODGE_UNIT first"
elif ! "$LODGECTL" status >/dev/null 2>&1; then
	problem "lodge is not up: $LODGECTL status fails"
else
	ok "lodge is installed and up ($LODGECTL)"
fi
if [ -f "$(p /usr/share/lodge/apps/notes/app.json)" ] && [ -f "$(p /usr/share/lodge/apps/notes/web/index.html)" ]; then
	ok "notes' manifest and web build are in /usr/share/lodge/apps/notes"
else
	problem "notes is not installed for lodge: copy build/lodge/apps/notes to /usr/share/lodge/apps/notes (make lodge-app)"
fi

app_pw=$("$GETENT" passwd "$APP_USER" || true)
if [ -n "$app_pw" ]; then
	app_uid=$(cut -d: -f3 <<<"$app_pw")
	app_gid=$(cut -d: -f4 <<<"$app_pw")
	ok "user $APP_USER exists (uid $app_uid, gid $app_gid)"
else
	problem "user $APP_USER does not exist: install packaging/sysusers.d/frostnotes.conf and run systemd-sysusers"
fi

for u in "$UNIT" "$SOCKET_UNIT"; do
	if [ "$(show "$u" LoadState)" = loaded ]; then ok "$u is installed"; else problem "$u is not installed (packaging/systemd/)"; fi
done
unit_was=$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null || true)
unit_enabled=$("$SYSTEMCTL" is-enabled "$UNIT" 2>/dev/null || true)
target=$(envof "$UNIT" FROSTNOTES_DATA_DIR)
target=${target:-/var/lib/frostnotes}
app_socket=$(envof "$UNIT" FROSTNOTES_SOCKET)
app_socket=${app_socket:-/run/frostnotes/notesd.sock}
exec_start=$(show "$UNIT" ExecStart | sed -n 's/.*path=\([^ ;]*\).*/\1/p')
notesctl=$(command -v notesctl || true)
if [ -n "$exec_start" ] && [ -x "$(p "$(dirname "$exec_start")/notesctl")" ]; then
	notesctl=$(p "$(dirname "$exec_start")/notesctl")
fi
if [ -z "$notesctl" ]; then
	problem "no notesctl beside ${exec_start:-notesd} or on PATH: install the one notesd came with"
elif ! "$notesctl" help hub 2>/dev/null | grep -qE '^[[:space:]]+url[[:space:]]'; then
	problem "$notesctl has no 'hub url' (an M2 notesctl?): install the one ${exec_start:-notesd} came with beside it"
fi
dropins=$(show "$UNIT" DropInPaths)
say "    $UNIT runs ${exec_start:-?} as $APP_USER, data $target, private socket $app_socket"
[ -z "$dropins" ] || say "    with drop-ins: $dropins (kept as they are: the binary, data and socket above are theirs)"

if [ "$(show "$TRIAL_UNIT" LoadState)" != loaded ]; then
	problem "$TRIAL_UNIT, the trial's hub, is not installed: nothing to move, or it moved already"
	trial_user=
else
	trial_user=$(show "$TRIAL_UNIT" User)
	trial_state=$("$SYSTEMCTL" is-active "$TRIAL_UNIT" 2>/dev/null || true)
	[ -n "$trial_user" ] || problem "$TRIAL_UNIT names no User=: which user's data is the hub's?"
	[ "$trial_user" != "$APP_USER" ] || problem "$TRIAL_UNIT runs as $APP_USER already"
	[ "$trial_state" = active ] || problem "$TRIAL_UNIT is $trial_state: start it, so its vaults and hub can be read before the move"
fi

trial_data=
trial_socket=
if [ -n "$trial_user" ]; then
	trial_home=$("$GETENT" passwd "$trial_user" | cut -d: -f6)
	# notesd's own defaults (internal/config): $XDG_DATA_HOME/frostnotes
	# and $XDG_RUNTIME_DIR/frostnotes/notesd.sock.
	trial_data=$(envof "$TRIAL_UNIT" FROSTNOTES_DATA_DIR)
	if [ -z "$trial_data" ]; then x=$(envof "$TRIAL_UNIT" XDG_DATA_HOME); trial_data=${x:-$trial_home/.local/share}/frostnotes; fi
	trial_socket=$(envof "$TRIAL_UNIT" FROSTNOTES_SOCKET)
	if [ -z "$trial_socket" ]; then x=$(envof "$TRIAL_UNIT" XDG_RUNTIME_DIR); trial_socket=${x:-/run}/frostnotes/notesd.sock; fi
	if [ -f "$(p "$trial_data")/notesd.db" ]; then
		ok "the hub's data is $trial_data ($(du -sh "$(p "$trial_data")" | cut -f1)), user $trial_user"
	else
		problem "$trial_data/notesd.db is missing: is $trial_user's notesd the hub?"
	fi
	if compgen -G "$(p "$trial_data").moved-*" >/dev/null; then
		problem "$(p "$trial_data").moved-* exists: this machine looks moved already (see /var/backups/frostnotes-move-*)"
	fi
fi

# The hub's ID and its vaults, from the running trial notesd, and the seq
# each of its vaults is at: a copy lodge's notesd took is in step at that.
hub_id=
declare -A hub_seq=()
vaults=()
if [ -n "$trial_user" ] && [ "${trial_state:-}" = active ]; then
	hub_id=$(as "$trial_user" env FROSTNOTES_SOCKET="$trial_socket" "$notesctl" hub status 2>/dev/null | sed -n 's/^hub \([0-9a-f]\{32\}\)$/\1/p')
	[ -n "$hub_id" ] && ok "the trial notesd is hub $hub_id" || problem "the trial notesd answers no hub ID: is it a hub?"
	while IFS=$'\t' read -r name _ seq; do
		[ -n "${name:-}" ] && [ -n "${seq:-}" ] && hub_seq[$name]=$seq
	done <<<"$(as "$trial_user" env FROSTNOTES_SOCKET="$trial_socket" "$notesctl" hub vaults 2>/dev/null || true)"
	while IFS=$'\t' read -r _ name path; do
		[ -n "${path:-}" ] && vaults+=("$path") && say "    vault $name at $path"
	done < <(as "$trial_user" env FROSTNOTES_SOCKET="$trial_socket" "$notesctl" vault list 2>/dev/null | grep -E $'^[0-9]+\t' || true)
	[ ${#vaults[@]} -gt 0 ] || problem "the trial notesd lists no vaults"
fi

# What frostnotes.service's data directory holds now.
aside=
aside_copies=
if [ -e "$(p "$target")" ] && [ -n "$(ls -A "$(p "$target")" 2>/dev/null)" ]; then
	if [ "$unit_was" != active ]; then
		problem "$target holds data and $UNIT is not running to say what: move it aside yourself, or start $UNIT"
	else
		their_hub=$(as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" hub status 2>/dev/null | sed -n 's/^hub \([0-9a-f]\{32\}\)$/\1/p')
		vault_rows=$(as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" vault list 2>/dev/null | grep -E $'^[0-9]+\t' || true)
		names=
		while IFS=$'\t' read -r _ name _; do [ -z "${name:-}" ] || names+="$name "; done <<<"$vault_rows"
		if [ -n "$hub_id" ] && [ "$their_hub" = "$hub_id" ]; then
			problem "$UNIT already serves hub $hub_id: the move looks done (see /var/backups/frostnotes-move-*)"
		elif [ "$names" = "Test " ]; then
			aside=$target.overnight-$STAMP
			ok "$target holds the overnight deployment's test (vault Test): it goes aside to $aside, never merged"
		elif [ -z "$names" ]; then
			problem "$target holds vaults other than the overnight test (none listed): move it aside yourself"
		else
			# lodge's notesd is a device of the trial hub: it keeps a synced
			# copy of each of the hub's vaults in its data directory, beside
			# the test. A copy the hub has in full, in step and verified
			# loses nothing by going aside.
			copies_ok=1
			while IFS=$'\t' read -r id name _; do
				if [ -z "${id:-}" ] || [ -z "${name:-}" ] || [ "$name" = Test ]; then continue; fi
				status=$(as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" sync status --vault "$id" 2>/dev/null || true)
				first=$(printf '%s\n' "$status" | awk -F'\t' -v id="$id" '$1 == id {print; exit}')
				side=$(printf '%s\n' "$status" | awk '/^\t/ {print; exit}')
				hname='' cur='' huburl='' pending='' conflicted=''
				[ -z "$first" ] || IFS=$'\t' read -r _ hname huburl _ cur pending conflicted <<<"$first"
				want=
				[ -z "$hname" ] || want=${hub_seq[$hname]:-}
				if [ -z "$first" ] || [ -z "$want" ]; then
					copies_ok=
					problem "vault $name in $target is not a copy of the hub's vaults: move $target aside yourself"
					continue
				fi
				reasons=()
				[ "$cur" = "$want/$want" ] || reasons+=("$cur cursor/hubSeq")
				[ "${pending%% *}" = 0 ] || reasons+=("${pending%% *} pending")
				[ "${conflicted%% *}" = 0 ] || reasons+=("${conflicted%% *} conflicted")
				[ -z "$side" ] || reasons+=("refused/skipped/error line")
				if [ ${#reasons[@]} -gt 0 ]; then
					copies_ok=
					list=$(printf '%s, ' "${reasons[@]}")
					problem "vault $name in $target, a copy of the hub's $hname, is not in step with the hub (${list%, }): let it sync, then run me again"
					continue
				fi
				if ! as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" sync verify --vault "$id" >/dev/null 2>&1; then
					copies_ok=
					problem "vault $name in $target does not verify against the hub"
					continue
				fi
				ok "vault $name in $target is a copy of the hub's vault $hname ($huburl), pushed in full and verified at seq $want"
			done <<<"$vault_rows"
			if [ -n "$copies_ok" ]; then
				aside=$target.overnight-$STAMP
				aside_copies=1
				ok "$target holds the overnight deployment's test and its copies of the hub's vaults: it goes aside to $aside, never merged"
			else
				problem "$target holds vaults other than the overnight test ($names): move it aside yourself"
			fi
		fi
	fi
fi

# The vaults' CIFS mounts in /etc/fstab: the longest mount point above each.
mounts=()
for v in "${vaults[@]}"; do
	best=
	while read -r _ mp type _; do
		[ "$type" = cifs ] || continue
		case $v/ in "${mp%/}"/*) [ ${#mp} -gt ${#best} ] && best=$mp ;; esac
	done < <(grep -Ev '^\s*(#|$)' "$(p /etc/fstab)")
	if [ -z "$best" ]; then
		problem "vault $v is under no cifs mount in /etc/fstab"
	else
		case " ${mounts[*]} " in
		*" $best "*) ;;
		*)
			mounts+=("$best")
			say "    vault mount $best: $(awk -v mp="$best" '$2 == mp {print $4}' "$(p /etc/fstab)")"
			;;
		esac
	fi
done

if [ "$mode" = apply ]; then
	[ -n "$url" ] || problem "--apply needs --url https://lodge.<tailnet>.ts.net/device/notes"
fi
case ${url:-https://} in https://*) ;; *) problem "--url must be https://" ;; esac
[ -z "$url" ] || [[ $url == */device/notes ]] || say "  ! $url does not end in /device/notes: is it lodge's device route?"

if [ ${#problems[@]} -gt 0 ]; then
	printf '\nmove-hub-behind-lodge: %d problem(s); nothing changed.\n' ${#problems[@]} >&2
	exit 1
fi

# ---- the move -------------------------------------------------------------

backup=$(p /var/backups)/frostnotes-move-$STAMP
step "Plan ($mode); backup and manifest in $backup"
run mkdir -p -m 0700 "$backup"
record() { if [ "$mode" = apply ]; then printf '%s=%s\n' "$1" "$2" >>"$backup/manifest"; fi; }
if [ "$mode" = apply ]; then
	trap '[ $? -eq 0 ] || printf "move-hub-behind-lodge: the move stopped; roll back with: %s --rollback %s --apply\n" "$0" "$backup" >&2' EXIT
fi
record target "$(p "$target")"
record trial_data "$(p "$trial_data")"
record unit_was "$unit_was"
record unit_enabled "$unit_enabled"
record mounts "${mounts[*]}"
record hub "$hub_id"
record app_socket "$app_socket"
record notesctl "$notesctl"

step "1. Stop and disable the trial's hub ($TRIAL_UNIT)"
run "$SYSTEMCTL" disable --now "$TRIAL_UNIT"

step "2. The hub's data into $target"
# The socket first: a connection to it would start notesd on the data
# directory while it is being replaced.
run "$SYSTEMCTL" stop "$SOCKET_UNIT" "$UNIT"
if [ -n "$aside" ]; then
	run mv "$(p "$target")" "$(p "$aside")"
	record aside "$(p "$aside")"
	[ -z "$aside_copies" ] || record aside_copies 1
fi
run rm -rf "$(p "$target").new"
run cp -a "$(p "$trial_data")" "$(p "$target").new"
run "$CHOWN" -R "$APP_USER:$APP_USER" "$(p "$target").new"
run chmod 0700 "$(p "$target").new"
if [ -z "$aside" ] && [ -e "$(p "$target")" ]; then run rmdir "$(p "$target")"; fi
run mv "$(p "$target").new" "$(p "$target")"
run mv "$(p "$trial_data")" "$(p "$trial_data").moved-$STAMP"
record trial_moved "$(p "$trial_data").moved-$STAMP"

step "3. The vaults' mounts, as $APP_USER"
run cp -p "$(p /etc/fstab)" "$backup/fstab"
for mp in "${mounts[@]}"; do
	say "  in /etc/fstab, $mp: uid= and gid= become ${app_uid:-?} and ${app_gid:-?}"
	if [ "$mode" = apply ]; then
		awk -v mp="$mp" -v uid="$app_uid" -v gid="$app_gid" '
			!/^[ \t]*#/ && $2 == mp {
				n = split($4, o, ","); out = ""; su = 0; sg = 0
				for (i = 1; i <= n; i++) {
					if (o[i] ~ /^uid=/) { o[i] = "uid=" uid; su = 1 }
					if (o[i] ~ /^gid=/) { o[i] = "gid=" gid; sg = 1 }
					out = out (i > 1 ? "," : "") o[i]
				}
				if (!su) out = out ",uid=" uid
				if (!sg) out = out ",gid=" gid
				$4 = out
			}
			{ print }' "$backup/fstab" >"$(p /etc/fstab).new"
		cp "$(p /etc/fstab).new" "$(p /etc/fstab)"
		rm "$(p /etc/fstab).new"
	fi
done
run "$SYSTEMCTL" daemon-reload
for mp in "${mounts[@]}"; do remount "$mp"; done

step "4. $APP_USER writes to each share"
for mp in "${mounts[@]}"; do
	check=$(p "$mp")/.frostnotes-write-check-$STAMP
	printf '  $ (as %s) write, read back and remove %s\n' "$APP_USER" "$check"
	if [ "$mode" = apply ]; then
		got=$(as "$APP_USER" sh -c 'printf ok >"$1" && cat "$1" && rm "$1"' sh "$check") || true
		[ "$got" = ok ] || fail "$APP_USER cannot write to $mp"
		say "  ✓ $APP_USER wrote to $mp"
	fi
done
say "  (Seen from another of the share's clients: a note a device saves after the move shows there too.)"

step "5. Start notes behind lodge"
run "$SYSTEMCTL" enable --now "$SOCKET_UNIT" "$UNIT"
[ "$mode" = apply ] || say "  then check that $UNIT serves hub $hub_id with the vaults above"
if [ "$mode" = apply ]; then
	for _ in $(seq 60); do
		[ "$("$SYSTEMCTL" is-active "$UNIT" 2>/dev/null)" = active ] && break
		sleep 0.5
	done
	now_hub=$(as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" hub status 2>/dev/null | sed -n 's/^hub \([0-9a-f]\{32\}\)$/\1/p')
	[ "$now_hub" = "$hub_id" ] || fail "$UNIT serves hub '${now_hub}', not $hub_id"
	say "  ✓ $UNIT serves hub $hub_id"
	listed=$(as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" vault list | cut -f3)
	for v in "${vaults[@]}"; do
		grep -qxF "$v" <<<"$listed" || fail "$UNIT does not list vault $v"
	done
	say "  ✓ its vaults are the hub's: ${vaults[*]}"
	# Where the hub's log is, for the rollback to tell whether devices
	# synced through lodge since.
	record seqs "$(as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" hub vaults | awk -F'\t' '{print $1 ":" $3}' | sort | tr '\n' ' ')"
fi

step "6. Devices move to lodge; lodge sees notes"
printf '  $ (as %s) notesctl hub url %s\n' "$APP_USER" "${url:-https://LODGE/device/notes}"
if [ "$mode" = apply ]; then
	as "$APP_USER" env FROSTNOTES_SOCKET="$app_socket" "$notesctl" hub url "$url"
	record url_set "$url"
fi
reload_lodge

step "Done"
say "Roll back with: $0 --rollback $backup --apply (before devices sync through lodge)"
say "notesctl for the hub: sudo -u $APP_USER FROSTNOTES_SOCKET=$app_socket $notesctl hub devices"
say "On each device: notesctl hub relocate OLD-URL ${url:-https://LODGE/device/notes}"
say "(OLD-URL is the hub the device synced with; the hub cannot tell it, as step 1 stopped it.)"
say "Once \`notesctl hub devices\` shows each device seen, the trial's old \`tailscale serve\` for the hub can go."
if [ -n "$aside_copies" ]; then
	say "Also revoke lodge's former device on the hub: notesctl hub revoke ID (the ID is in notesctl hub devices)."
fi
[ "$mode" = apply ] || say $'\nDry run: nothing changed. Run again with --apply --url … to move.'
