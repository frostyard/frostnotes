//! Why notesd is not running, for the window that cannot reach it
//! (docs/design/app.md#the-flatpak). In the Flatpak the launcher
//! (packaging/flatpak/frostnotes.sh) has the Flatpak portal start notesd,
//! and when the portal does not, it appends one line to notesd.log; the
//! window would only say it is reconnecting. This reads the end of that log
//! so the window can say why. Nothing else reads the file: it is notesd's.

use serde::Serialize;
use std::io::{Read, Seek, SeekFrom};
use std::path::Path;

use crate::paths;

/// TAIL is how much of the log's end is read: a bounded read, whatever the
/// log's size (notesd turns a log over 1 MiB into notesd.log.1).
const TAIL: u64 = 8 * 1024;

/// PORTAL_FAILURE starts the line the launcher appends when no notesd
/// answers after flatpak-spawn. The launcher's own line continues with
/// "; see $state/notesd.log"; a test pins this prefix to the launcher.
const PORTAL_FAILURE: &str = "frostnotes: notesd did not start through the Flatpak portal (flatpak-spawn)";

/// NotesdLog is what the window learns from notesd.log.
#[derive(Debug, PartialEq, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct NotesdLog {
    /// The log's path.
    pub path: String,
    /// Its last line that holds text; none when the log is empty or missing.
    pub last_line: Option<String>,
    /// That line is the launcher's: the Flatpak portal did not start notesd.
    pub portal_refused: bool,
}

/// last_line is the last line of text that holds more than white space.
fn last_line(text: &str) -> Option<String> {
    text.lines().rev().map(str::trim).find(|l| !l.is_empty()).map(str::to_string)
}

/// tail reads at most the last TAIL bytes of the file at path.
fn tail(path: &Path) -> std::io::Result<String> {
    let mut file = std::fs::File::open(path)?;
    let len = file.metadata()?.len();
    file.seek(SeekFrom::Start(len.saturating_sub(TAIL)))?;
    let mut bytes = Vec::new();
    file.take(TAIL).read_to_end(&mut bytes)?;
    // The cut may fall inside a character; only the last line is used.
    Ok(String::from_utf8_lossy(&bytes).into_owned())
}

/// describe reads the log at path; a log that cannot be read has no last line.
fn describe(path: &Path) -> NotesdLog {
    let last_line = tail(path).ok().and_then(|text| last_line(&text));
    NotesdLog {
        path: path.display().to_string(),
        portal_refused: last_line.as_deref().is_some_and(|l| l.starts_with(PORTAL_FAILURE)),
        last_line,
    }
}

/// notesd_log reports notesd.log's end in the Flatpak, and nothing
/// anywhere else: a notesd that a person runs themselves has its own log.
#[tauri::command]
pub fn notesd_log() -> Option<NotesdLog> {
    if !paths::in_flatpak() {
        return None;
    }
    Some(describe(&paths::notesd_log().ok()?))
}

#[cfg(test)]
mod tests {
    use super::{PORTAL_FAILURE, TAIL, describe, last_line, tail};
    use std::fs;

    fn scratch(name: &str) -> std::path::PathBuf {
        let dir = std::env::temp_dir().join(format!("frostnotes-log-{name}-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        dir
    }

    // The launcher is a shell script that another change may rewrite: its
    // line must stay the one the window recognises.
    #[test]
    fn the_prefix_is_the_launchers_line() {
        let launcher = include_str!("../../../packaging/flatpak/frostnotes.sh");
        let line = format!("msg=\"{PORTAL_FAILURE}; see $state/notesd.log\"");
        assert!(launcher.contains(&line), "frostnotes.sh no longer holds {line}");
    }

    #[test]
    fn takes_the_last_line_with_text() {
        assert_eq!(last_line("one\ntwo\n"), Some("two".to_string()));
        assert_eq!(last_line("one\n  two  \n\n \n"), Some("two".to_string()));
        assert_eq!(last_line("one\r\ntwo\r\n"), Some("two".to_string()));
        assert_eq!(last_line("\n \n"), None);
        assert_eq!(last_line(""), None);
    }

    #[test]
    fn recognises_the_portal_failure() {
        let dir = scratch("portal");
        let log = dir.join("notesd.log");
        let line = format!("{PORTAL_FAILURE}; see {}/notesd.log", dir.display());
        fs::write(&log, format!("notesd: listening\n{line}\n")).unwrap();
        let got = describe(&log);
        assert_eq!(got.last_line.as_deref(), Some(line.as_str()));
        assert!(got.portal_refused);
        assert_eq!(got.path, log.display().to_string());

        // Later lines are notesd's own: the portal worked since.
        fs::write(&log, format!("{line}\nnotesd: listening on /run/x\n")).unwrap();
        let got = describe(&log);
        assert_eq!(got.last_line.as_deref(), Some("notesd: listening on /run/x"));
        assert!(!got.portal_refused);
        fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn a_missing_or_empty_log_has_no_last_line() {
        let dir = scratch("none");
        let log = dir.join("notesd.log");
        let got = describe(&log);
        assert_eq!((got.last_line, got.portal_refused), (None, false));
        assert_eq!(got.path, log.display().to_string());
        fs::write(&log, "").unwrap();
        assert_eq!(describe(&log).last_line, None);
        fs::remove_dir_all(&dir).unwrap();
    }

    #[test]
    fn reads_only_the_end_of_a_long_log() {
        let dir = scratch("long");
        let log = dir.join("notesd.log");
        let filler = "a long line of an old log\n".repeat(2000); // about 52 KB
        assert!(filler.len() as u64 > 4 * TAIL);
        fs::write(&log, format!("{filler}the end\n")).unwrap();
        assert!(tail(&log).unwrap().len() as u64 <= TAIL);
        assert_eq!(describe(&log).last_line.as_deref(), Some("the end"));
        // A cut inside a multi-byte character does not fail the read.
        fs::write(&log, format!("{}\nthe end\n", "é".repeat(TAIL as usize))).unwrap();
        assert_eq!(describe(&log).last_line.as_deref(), Some("the end"));
        fs::remove_dir_all(&dir).unwrap();
    }
}
