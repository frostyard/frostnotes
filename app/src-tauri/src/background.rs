//! The Flatpak's setting for background activity, and the stop it asks for
//! (docs/adr/0033-a-setting-keeps-notesd-running-without-a-window.md,
//! docs/design/app.md#the-flatpak). Off is a file,
//! `$XDG_CONFIG_HOME/frostnotes/background-off`, which the launcher
//! (packaging/flatpak/frostnotes.sh) and this shell both read; On is its
//! absence. In the Flatpak, `XDG_CONFIG_HOME` is the app's own config
//! directory, which all of its sandboxes share.
//!
//! Off means three things: the launcher's request helper reads the file and
//! asks the Background portal for no autostart, so the portal removes the
//! autostart entry; `frostnotes --background`, the autostart's command line,
//! starts no notesd at a login; and this shell stops notesd when the app
//! exits. The launcher cannot do the last one: it cannot tell the launch a
//! person made from the second launch that tauri-plugin-single-instance
//! hands over and ends.
//!
//! The portal's last answer is the helper's, not the shell's:
//! `frostnotes-request-background` writes `$XDG_STATE_HOME/frostnotes/background.json`
//! (packaging/flatpak/request-background.py), and `background_get` reads it,
//! so the window can show whether the desktop allows background activity.

use std::ffi::OsString;
use std::os::unix::process::CommandExt;
use std::path::PathBuf;
use std::process::{Command, Stdio};
use std::sync::atomic::{AtomicBool, Ordering};
use std::time::{Duration, Instant};

use serde::{Deserialize, Serialize};

use crate::paths;

/// DIR holds the setting's files, as the launcher's `$state` does.
const DIR: &str = "frostnotes";

/// OFF_FILE is the setting's file: it exists while the setting is Off.
const OFF_FILE: &str = "background-off";

/// ANSWER_FILE holds the Background portal's last answer, written by the
/// helper.
const ANSWER_FILE: &str = "background.json";

/// LOG_FILE is the helper's output, as the launcher's request writes it.
const LOG_FILE: &str = "background.log";

/// HELPER is the launcher's Background-portal helper.
const HELPER: &str = "frostnotes-request-background";

/// LIBEXEC is the Flatpak's directory of `frostnotes-app` and the helper;
/// FROSTNOTES_LIBEXEC_DIR names another one (the launcher's own override).
const LIBEXEC: &str = "/app/libexec";

/// SHUTDOWN_WAIT bounds `notesctl shutdown` at the exit: its own limit is
/// 30 s.
const SHUTDOWN_WAIT: Duration = Duration::from_secs(35);

/// POLL is how often the exit checks whether `notesctl` has returned.
const POLL: Duration = Duration::from_millis(100);

/// PortalAnswer is the Background portal's last answer, as the helper wrote
/// it to background.json.
#[derive(Debug, Default, Deserialize, PartialEq, Serialize)]
#[serde(rename_all = "camelCase", default)]
pub struct PortalAnswer {
    /// When the helper asked, in RFC 3339 UTC time.
    pub at: String,
    /// Whether the request asked for autostart at login; false with the
    /// setting Off, which makes the portal remove the autostart entry.
    pub autostart_requested: bool,
    /// The response: 0 granted, 1 cancelled, 2 other; none when the helper
    /// got no answer.
    pub response: Option<u8>,
    /// The portal's background permission.
    pub background: Option<bool>,
    /// The portal's autostart permission.
    pub autostart: Option<bool>,
    /// Why the request failed, when it did.
    pub error: Option<String>,
}

/// Background is the setting and the portal's last answer, for the window.
#[derive(Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct Background {
    /// On: notesd keeps syncing after the last window closes, and the
    /// portal may start it at login.
    pub keep_running: bool,
    /// The portal's last answer; none before the first, or when the file
    /// cannot be read.
    pub portal: Option<PortalAnswer>,
}

/// background_get reports the setting in the Flatpak, and nothing anywhere
/// else: outside it, notesd is a service that the desktop and systemd run.
#[tauri::command]
pub fn background_get() -> Option<Background> {
    if !paths::in_flatpak() {
        return None;
    }
    let getenv = |key: &str| std::env::var_os(key);
    Some(state(&getenv))
}

/// background_set writes the setting, asks the Background portal again, and
/// answers the new state.
#[tauri::command]
pub fn background_set(keep_running: bool) -> Result<Background, String> {
    if !paths::in_flatpak() {
        return Err("the background setting belongs to the Flatpak".to_string());
    }
    let getenv = |key: &str| std::env::var_os(key);
    set(keep_running, &getenv)
}

/// set creates or removes the setting's file, then starts the helper, which
/// reads the file itself: the portal then writes the autostart entry again
/// when the setting is On, and removes it when Off.
fn set(
    keep_running: bool,
    getenv: &impl Fn(&str) -> Option<OsString>,
) -> Result<Background, String> {
    let off = off_file(getenv)?;
    if keep_running {
        match std::fs::remove_file(&off) {
            Ok(()) => {}
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => return Err(format!("remove {}: {e}", off.display())),
        }
    } else {
        let dir = off.parent().ok_or_else(|| format!("{} has no directory", off.display()))?;
        std::fs::create_dir_all(dir).map_err(|e| format!("create {}: {e}", dir.display()))?;
        let text = "Off: notesd stops when the last window closes (ADR-0033).\n";
        std::fs::write(&off, text).map_err(|e| format!("write {}: {e}", off.display()))?;
    }
    ask(getenv);
    Ok(state(getenv))
}

/// state reads the setting and the portal's last answer.
fn state(getenv: &impl Fn(&str) -> Option<OsString>) -> Background {
    let off = off_file(getenv).is_ok_and(|p| p.exists());
    let portal = state_dir(getenv)
        .ok()
        .and_then(|d| std::fs::read_to_string(d.join(ANSWER_FILE)).ok())
        .and_then(|text| parse_portal(&text));
    Background { keep_running: !off, portal }
}

/// ask starts the helper in the background, with no arguments and its output
/// appended to background.log, as the launcher's own request does. Nothing
/// waits for it: the helper stays on the bus until the portal answers, at
/// most 10 minutes. It reads the setting itself, and its lock makes it and
/// the launcher's request take turns. A helper that cannot start changes
/// nothing: the next launch of the app asks again.
fn ask(getenv: &impl Fn(&str) -> Option<OsString>) {
    let Ok(dir) = state_dir(getenv) else {
        return;
    };
    if std::fs::create_dir_all(&dir).is_err() {
        return;
    }
    let opened = std::fs::OpenOptions::new().create(true).append(true).open(dir.join(LOG_FILE));
    let Ok(log) = opened else {
        return;
    };
    let Ok(errlog) = log.try_clone() else {
        return;
    };
    let mut command = Command::new(helper(getenv));
    command.stdin(Stdio::null()).stdout(log).stderr(errlog);
    // A process group of its own, as the launcher's `&` leaves it: the
    // app's signals do not reach the request.
    let _ = command.process_group(0);
    if let Ok(mut child) = command.spawn() {
        // Reap it, so that a helper still waiting for the portal leaves no
        // zombie behind.
        std::thread::spawn(move || {
            let _ = child.wait();
        });
    }
}

/// PRIMARY is true in the process that owns the app's windows. A second
/// launch hands over to it and ends in tauri-plugin-single-instance, before
/// the setup closure that marks it runs.
static PRIMARY: AtomicBool = AtomicBool::new(false);

/// mark_primary records that this process owns the app's windows.
pub fn mark_primary() {
    PRIMARY.store(true, Ordering::SeqCst);
}

/// stop_on_exit stops notesd when the app exits with the setting Off, so
/// that Off holds also where the desktop's own background permission is on.
/// Only the primary instance does this, so that a second launch ends no
/// notesd, and only in the Flatpak: elsewhere notesd is a service of its
/// own. A spike run does not mark itself primary, so it stops no notesd.
pub fn stop_on_exit() {
    if !PRIMARY.load(Ordering::SeqCst) || !paths::in_flatpak() {
        return;
    }
    let getenv = |key: &str| std::env::var_os(key);
    if !off_file(&getenv).is_ok_and(|p| p.exists()) {
        return;
    }
    shutdown();
}

/// shutdown runs `notesctl shutdown`, the launcher's own way to stop
/// notesd, and waits for it at most SHUTDOWN_WAIT. A notesctl that still
/// runs after that, or one that cannot start, changes nothing: notesd then
/// stops as it did before this setting.
fn shutdown() {
    let spawned = Command::new("notesctl")
        .arg("shutdown")
        .stdin(Stdio::null())
        .stdout(Stdio::null())
        .stderr(Stdio::null())
        .spawn();
    let Ok(mut child) = spawned else {
        return;
    };
    let deadline = Instant::now() + SHUTDOWN_WAIT;
    loop {
        match child.try_wait() {
            Ok(Some(_)) => return,
            Ok(None) if Instant::now() < deadline => std::thread::sleep(POLL),
            // It still runs at the deadline, or its state cannot be read:
            // end it either way.
            _ => break,
        }
    }
    let _ = child.kill();
    let _ = child.wait();
}

/// config_dir is $XDG_CONFIG_HOME, else ~/.config: in the Flatpak, the
/// app's own config directory, which all of its sandboxes share.
fn config_dir(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    match abs(getenv("XDG_CONFIG_HOME")) {
        Some(dir) => Ok(dir),
        None => Ok(home(getenv)?.join(".config")),
    }
}

/// state_dir is $XDG_STATE_HOME, else ~/.local/state, with the launcher's
/// `frostnotes` under it.
fn state_dir(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    match abs(getenv("XDG_STATE_HOME")) {
        Some(dir) => Ok(dir.join(DIR)),
        None => Ok(home(getenv)?.join(".local").join("state").join(DIR)),
    }
}

/// off_file is the setting's file: the launcher reads it, and so does this
/// shell.
fn off_file(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    Ok(config_dir(getenv)?.join(DIR).join(OFF_FILE))
}

/// helper is the Background-portal helper: $FROSTNOTES_LIBEXEC_DIR's, else
/// /app/libexec's, as the launcher finds it.
fn helper(getenv: impl Fn(&str) -> Option<OsString>) -> PathBuf {
    match abs(getenv("FROSTNOTES_LIBEXEC_DIR")) {
        Some(dir) => dir.join(HELPER),
        None => PathBuf::from(LIBEXEC).join(HELPER),
    }
}

/// home is $HOME, the base of the XDG defaults.
fn home(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    getenv("HOME")
        .filter(|v| !v.is_empty())
        .map(PathBuf::from)
        .ok_or_else(|| "HOME is not set".to_string())
}

/// abs is an XDG base directory: an absolute path, as paths.rs reads them.
fn abs(value: Option<OsString>) -> Option<PathBuf> {
    value.map(PathBuf::from).filter(|p| p.is_absolute())
}

/// parse_portal reads the helper's answer. A file that is not a JSON
/// object, or that holds a value of another type, is no answer; a key it
/// lacks keeps its default, since the helper may add keys later.
fn parse_portal(text: &str) -> Option<PortalAnswer> {
    serde_json::from_str(text).ok()
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;

    fn env<'a>(vars: &'a [(&'a str, &'a str)]) -> impl Fn(&str) -> Option<OsString> + 'a {
        move |key| vars.iter().find(|(k, _)| *k == key).map(|(_, v)| OsString::from(v))
    }

    fn scratch(name: &str) -> PathBuf {
        let name = format!("frostnotes-background-{name}-{}", std::process::id());
        let dir = std::env::temp_dir().join(name);
        let _ = fs::remove_dir_all(&dir);
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    #[test]
    fn the_setting_lives_in_the_config_directory() {
        let under_home = Ok(PathBuf::from("/home/u/.config/frostnotes/background-off"));
        let vars = [("XDG_CONFIG_HOME", "/cfg"), ("HOME", "/home/u")];
        assert_eq!(off_file(env(&vars)), Ok(PathBuf::from("/cfg/frostnotes/background-off")));
        // A relative base is no base, as in paths.rs.
        let vars = [("XDG_CONFIG_HOME", "cfg"), ("HOME", "/home/u")];
        assert_eq!(off_file(env(&vars)), under_home);
        let vars = [("HOME", "/home/u")];
        assert_eq!(off_file(env(&vars)), under_home);
        assert!(off_file(env(&[])).is_err());
    }

    #[test]
    fn the_state_directory_follows_the_launcher() {
        let vars = [("XDG_STATE_HOME", "/state"), ("HOME", "/home/u")];
        assert_eq!(state_dir(env(&vars)), Ok(PathBuf::from("/state/frostnotes")));
        let vars = [("XDG_STATE_HOME", ""), ("HOME", "/home/u")];
        assert_eq!(state_dir(env(&vars)), Ok(PathBuf::from("/home/u/.local/state/frostnotes")));
        assert!(state_dir(env(&[])).is_err());
    }

    #[test]
    fn the_helper_is_the_flatpaks_or_the_override() {
        let helper_name = "frostnotes-request-background";
        let vars = [("FROSTNOTES_LIBEXEC_DIR", "/test/libexec")];
        assert_eq!(helper(env(&vars)), PathBuf::from("/test/libexec").join(helper_name));
        let vars = [("FROSTNOTES_LIBEXEC_DIR", "libexec")];
        assert_eq!(helper(env(&vars)), PathBuf::from("/app/libexec").join(helper_name));
        assert_eq!(helper(env(&[])), PathBuf::from("/app/libexec").join(helper_name));
    }

    #[test]
    fn the_setting_is_the_off_files_absence() {
        let dir = scratch("state");
        let base = dir.to_str().unwrap();
        let vars = [("XDG_CONFIG_HOME", base), ("XDG_STATE_HOME", base)];
        let get = env(&vars);
        let got = state(&get);
        assert!(got.keep_running);
        assert_eq!(got.portal, None);
        let off = off_file(&get).unwrap();
        fs::create_dir_all(off.parent().unwrap()).unwrap();
        fs::write(&off, "Off\n").unwrap();
        assert!(!state(&get).keep_running);
        fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn set_writes_and_removes_the_off_file() {
        let dir = scratch("set");
        let base = dir.to_str().unwrap();
        // A helper directory that does not exist: the request cannot start,
        // and the setting still changes.
        let vars = [
            ("XDG_CONFIG_HOME", base),
            ("XDG_STATE_HOME", base),
            ("FROSTNOTES_LIBEXEC_DIR", "/nonexistent/libexec"),
        ];
        let get = env(&vars);
        let off = off_file(&get).unwrap();
        assert!(!set(false, &get).unwrap().keep_running);
        assert!(off.exists());
        assert!(set(true, &get).unwrap().keep_running);
        assert!(!off.exists());
        // Removing a file that is not there is not an error.
        assert!(set(true, &get).is_ok());
        fs::remove_dir_all(&dir).unwrap();
    }

    /// json builds an object the way the helper writes it: no spaces.
    fn json(pairs: &[(&str, &str)]) -> String {
        let fields: Vec<String> = pairs
            .iter()
            .map(|(key, value)| format!("\"{key}\":{value}"))
            .collect();
        format!("{{{}}}", fields.join(","))
    }

    #[test]
    fn reads_the_helpers_answer() {
        let text = json(&[
            ("at", "\"2026-10-28T09:00:00Z\""),
            ("autostartRequested", "true"),
            ("response", "0"),
            ("background", "true"),
            ("autostart", "true"),
            ("error", "null"),
        ]);
        let want = PortalAnswer {
            at: "2026-10-28T09:00:00Z".to_string(),
            autostart_requested: true,
            response: Some(0),
            background: Some(true),
            autostart: Some(true),
            error: None,
        };
        assert_eq!(parse_portal(&text), Some(want));
        // White space between the fields changes nothing.
        assert_eq!(parse_portal(&text.replace(',', ", ")), parse_portal(&text));
        let off = json(&[
            ("at", "\"2026-10-28T09:00:00Z\""),
            ("autostartRequested", "false"),
            ("response", "0"),
            ("background", "false"),
            ("autostart", "false"),
            ("error", "null"),
        ]);
        let got = parse_portal(&off).unwrap();
        assert_eq!(
            (got.autostart_requested, got.background, got.autostart),
            (false, Some(false), Some(false))
        );
    }

    #[test]
    fn reads_a_partial_answer() {
        let text = r#"{"at":"2026-10-28T09:00:00Z","response":2,"error":"no"}"#;
        let got = parse_portal(text).unwrap();
        assert_eq!((got.response, got.background, got.autostart), (Some(2), None, None));
        assert_eq!(got.error.as_deref(), Some("no"));
        assert!(!got.autostart_requested);
        // A file with no key read here keeps every default: no answer yet.
        assert_eq!(parse_portal("{}"), Some(PortalAnswer::default()));
        assert_eq!(parse_portal(r#"{"unknown": 1}"#), Some(PortalAnswer::default()));
    }

    #[test]
    fn a_bad_file_is_no_answer() {
        for text in ["", "null", "[1]", r#"{"background": "yes"}"#, r#"{"response": 300}"#] {
            assert_eq!(parse_portal(text), None, "{text:?}");
        }
    }
}
