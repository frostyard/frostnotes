//! File locations, mirroring internal/config: the override, then the XDG
//! base directory.

use std::ffi::OsString;
use std::path::PathBuf;

fn abs(value: Option<OsString>) -> Option<PathBuf> {
    value.map(PathBuf::from).filter(|p| p.is_absolute())
}

/// notesd's socket: $FROSTNOTES_SOCKET or $XDG_RUNTIME_DIR/frostnotes/notesd.sock.
pub fn socket() -> Result<PathBuf, String> {
    socket_from(|key| std::env::var_os(key))
}

fn socket_from(getenv: impl Fn(&str) -> Option<OsString>) -> Result<PathBuf, String> {
    if let Some(p) = abs(getenv("FROSTNOTES_SOCKET")) {
        return Ok(p);
    }
    abs(getenv("XDG_RUNTIME_DIR"))
        .map(|d| d.join("frostnotes").join("notesd.sock"))
        .ok_or_else(|| "XDG_RUNTIME_DIR is not set; set FROSTNOTES_SOCKET".to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

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
    fn socket_needs_an_absolute_runtime_dir() {
        assert!(socket_from(env(&[])).is_err());
        assert!(socket_from(env(&[("XDG_RUNTIME_DIR", "run")])).is_err());
    }
}
