//! File locations, mirroring internal/config: the override, then the XDG
//! base directory.

use std::ffi::OsString;
use std::path::PathBuf;

/// The application ID, as internal/config.AppID.
const APP_ID: &str = "org.frostyard.Frostnotes";

fn abs(value: Option<OsString>) -> Option<PathBuf> {
    value.map(PathBuf::from).filter(|p| p.is_absolute())
}

/// notesd's socket: $FROSTNOTES_SOCKET or $XDG_RUNTIME_DIR/frostnotes/notesd.sock.
/// In the Flatpak (FLATPAK_ID is the app ID) it is
/// $XDG_RUNTIME_DIR/app/org.frostyard.Frostnotes/frostnotes/notesd.sock, the one
/// directory Flatpak shares between the app's sandboxes. Keep in step with
/// config.DefaultSocket in internal/config.
pub fn socket() -> Result<PathBuf, String> {
    socket_from(|key| std::env::var_os(key))
}

fn socket_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = abs(getenv("FROSTNOTES_SOCKET")) {
        return Ok(p);
    }
    abs(getenv("XDG_RUNTIME_DIR"))
        .map(|d| {
            if getenv("FLATPAK_ID").is_some_and(|id| id == APP_ID) {
                d.join("app").join(APP_ID).join("frostnotes").join("notesd.sock")
            } else {
                d.join("frostnotes").join("notesd.sock")
            }
        })
        .ok_or_else(|| "XDG_RUNTIME_DIR is not set; set FROSTNOTES_SOCKET".to_string())
}

/// notesd's data directory, where its blob store is, as internal/config
/// resolves it: $FROSTNOTES_DATA_DIR, else $XDG_DATA_HOME/frostnotes,
/// else ~/.local/share/frostnotes.
pub fn data_dir() -> Result<PathBuf, String> {
    data_dir_from(|key| std::env::var_os(key))
}

fn data_dir_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = getenv("FROSTNOTES_DATA_DIR").filter(|v| !v.is_empty()) {
        return Ok(PathBuf::from(p));
    }
    if let Some(p) = abs(getenv("XDG_DATA_HOME")) {
        return Ok(p.join("frostnotes"));
    }
    getenv("HOME")
        .filter(|v| !v.is_empty())
        .map(|h| PathBuf::from(h).join(".local").join("share").join("frostnotes"))
        .ok_or_else(|| "HOME is not set; set FROSTNOTES_DATA_DIR".to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn data_dir_follows_internal_config() {
        let vars = [("FROSTNOTES_DATA_DIR", "/data/fn"), ("XDG_DATA_HOME", "/xdg"), ("HOME", "/home/u")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/data/fn")));
        let vars = [("XDG_DATA_HOME", "/xdg"), ("HOME", "/home/u")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/xdg/frostnotes")));
        let vars = [("XDG_DATA_HOME", "relative"), ("HOME", "/home/u")];
        assert_eq!(data_dir_from(env(&vars)), Ok(PathBuf::from("/home/u/.local/share/frostnotes")));
        assert!(data_dir_from(env(&[])).is_err());
    }

    fn env<'a>(vars: &'a [(&'a str, &'a str)]) -> impl Fn(&str) -> Option<OsString> + 'a {
        move |key| vars.iter().find(|(k, _)| *k == key).map(|(_, v)| OsString::from(v))
    }

    #[test]
    fn socket_prefers_the_override() {
        let vars = [("FROSTNOTES_SOCKET", "/tmp/n.sock"), ("XDG_RUNTIME_DIR", "/run/user/1000")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/tmp/n.sock")));
    }

    #[test]
    fn socket_defaults_to_the_runtime_dir() {
        let vars = [("FROSTNOTES_SOCKET", "relative.sock"), ("XDG_RUNTIME_DIR", "/run/user/1000")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/run/user/1000/frostnotes/notesd.sock")));
    }

    #[test]
    fn socket_in_the_flatpak_is_in_the_shared_app_directory() {
        let vars = [("XDG_RUNTIME_DIR", "/run/user/1000"), ("FLATPAK_ID", "org.frostyard.Frostnotes")];
        assert_eq!(
            socket_from(env(&vars)),
            Ok(PathBuf::from("/run/user/1000/app/org.frostyard.Frostnotes/frostnotes/notesd.sock"))
        );
        let vars = [("XDG_RUNTIME_DIR", "/run/user/1000"), ("FLATPAK_ID", "org.example.Other")];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/run/user/1000/frostnotes/notesd.sock")));
        let vars = [
            ("FROSTNOTES_SOCKET", "/tmp/n.sock"),
            ("XDG_RUNTIME_DIR", "/run/user/1000"),
            ("FLATPAK_ID", "org.frostyard.Frostnotes"),
        ];
        assert_eq!(socket_from(env(&vars)), Ok(PathBuf::from("/tmp/n.sock")));
    }

    #[test]
    fn socket_needs_an_absolute_runtime_dir() {
        assert!(socket_from(env(&[])).is_err());
        assert!(socket_from(env(&[("XDG_RUNTIME_DIR", "run")])).is_err());
    }
}
