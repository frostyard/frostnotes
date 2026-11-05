//! Problem records: the shell's panics go to the local systemd journal,
//! and nowhere else (docs/adr/0036-notesd-and-the-app-record-problems-in-the-journal.md).
//! The record holds the app's ID and version, the runtime and the
//! component, and the panic's message and its place under `src/`; the
//! crate `frostyard-problems` writes it as 1 datagram to the journal's
//! socket. Nothing is sent over a network.

use frostyard_problems::{Component, Recorder};

/// The app ID of every Frostnotes record.
const APP_ID: &str = "org.frostyard.Frostnotes";

/// The version of a build that no release stamped (tauri.conf.json's).
const UNSTAMPED: &str = "0.0.0";

/// The shell's recorder at `version`, or `None` for an unstamped build,
/// which records nothing.
fn recorder(version: &str) -> Option<Recorder> {
    (version != UNSTAMPED).then(|| Recorder::new(APP_ID, version, Component::Gui))
}

/// Installs the panic hook that records each panic of the shell, then
/// calls the hook that was there before. `version` is the app's version
/// (`package_info().version`): the release's on a tagged Flatpak build.
/// An unstamped build, or a version that is not a release semver, installs
/// nothing. Call it early in `main`.
pub fn install(version: &str) -> Option<Recorder> {
    let rec = recorder(version)?;
    if let Some(why) = rec.err() {
        eprintln!("frostnotes: problem records are off: {why}");
        return None;
    }
    hook(&rec);
    Some(rec)
}

/// Installs `rec`'s panic hook from this crate, so that a panic's place in
/// the shell's `src/` is kept (frostyard-problems' `install_panic_hook`).
fn hook(rec: &Recorder) {
    rec.install_panic_hook(env!("CARGO_MANIFEST_DIR"));
}

#[cfg(test)]
mod tests {
    use super::{APP_ID, hook, install, recorder};
    use frostyard_problems::{decode_entry, validate};
    use std::os::unix::net::UnixDatagram;
    use std::panic;
    use std::path::PathBuf;
    use std::sync::Mutex;
    use std::time::Duration;

    /// Serializes the tests that change the process's panic hook.
    static HOOK: Mutex<()> = Mutex::new(());

    /// A datagram socket in a folder of its own, in place of the journal's.
    struct Journal {
        dir: PathBuf,
        path: PathBuf,
        sock: UnixDatagram,
    }

    impl Journal {
        fn new(name: &str) -> Journal {
            let dir = std::env::temp_dir().join(format!("frostnotes-problems-{}-{name}", std::process::id()));
            let _ = std::fs::remove_dir_all(&dir);
            std::fs::create_dir(&dir).unwrap();
            let path = dir.join("journal.sock");
            let sock = UnixDatagram::bind(&path).unwrap();
            sock.set_read_timeout(Some(Duration::from_secs(10))).unwrap();
            Journal { dir, path, sock }
        }

        /// The next record's fields.
        fn next(&self) -> std::collections::HashMap<String, String> {
            let mut buf = vec![0; 1 << 20];
            let n = self.sock.recv(&mut buf).expect("a record");
            decode_entry(&buf[..n]).expect("a record the journal decodes")
        }

        /// Fails when the socket holds a record: a record is sent before
        /// the panic hook returns, so it is read without waiting.
        fn none(&self) {
            self.sock.set_nonblocking(true).unwrap();
            let mut buf = vec![0; 1 << 20];
            if let Ok(n) = self.sock.recv(&mut buf) {
                panic!("an unexpected record: {:?}", String::from_utf8_lossy(&buf[..n]));
            }
        }
    }

    impl Drop for Journal {
        fn drop(&mut self) {
            let _ = std::fs::remove_dir_all(&self.dir);
        }
    }

    // A panic of the shell is 1 record that validate accepts: the app, its
    // version, the Rust runtime, the gui component, class panic and the
    // panic's place in src/ (ADR-0036).
    #[test]
    fn a_panic_is_recorded() {
        let _serial = HOOK.lock().unwrap_or_else(|e| e.into_inner());
        let journal = Journal::new("panic");
        let rec = recorder("1.2.3").expect("a recorder at 1.2.3").with_socket(&journal.path);
        assert_eq!(rec.err(), None);
        let prev = panic::take_hook();
        panic::set_hook(Box::new(|_| {})); // the test's output stays quiet
        hook(&rec);
        let caught = panic::catch_unwind(|| panic!("a bug in the shell"));
        let _ = panic::take_hook();
        panic::set_hook(prev);
        assert!(caught.is_err());

        let fields = journal.next();
        let detail = validate(&fields).expect("a valid record");
        assert_eq!(detail.app_id, APP_ID);
        assert_eq!(detail.app_version, "1.2.3");
        assert_eq!(detail.runtime, "rust");
        assert_eq!(detail.component, "gui");
        assert_eq!(detail.class, "panic");
        assert_eq!(detail.code, None);
        let location = detail.location.expect("a location");
        assert!(location.starts_with("src/problems.rs:"), "location {location}");
        assert_eq!(fields.get("MESSAGE").map(String::as_str), Some("a bug in the shell"));
        journal.none();
    }

    // An unstamped build (version 0.0.0) has no recorder and installs no
    // hook; neither does a version that is not a release.
    #[test]
    fn an_unstamped_build_records_nothing() {
        let _serial = HOOK.lock().unwrap_or_else(|e| e.into_inner());
        assert!(recorder("0.0.0").is_none());
        assert!(install("0.0.0").is_none());
        assert!(install("0.1.0-dev").is_none());
    }
}
