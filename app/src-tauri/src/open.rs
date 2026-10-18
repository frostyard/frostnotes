//! Opening things outside the app, the only two ways the webview reaches
//! the system (docs/design/app.md#window): a web link in the system's
//! browser (http, https and mailto only), and a vault file in the system's
//! default app (Open in default app). Both checks are here, in the shell,
//! not in the page: a file must be a regular, non-executable file inside
//! the vault folder it names, of a type that opens as a document, never
//! one a desktop runs (a script, a .desktop entry, HTML). The system opens
//! them with xdg-open, the path or URL one argument, no shell. The
//! extension list guards a file's type, not the handler: xdg-open may pick
//! one by the file's content. A vault on a CIFS mount whose file_mode has
//! execute bits (the kernel's default, 0755) shows every file executable,
//! so Open in default app refuses them all; mount it with file_mode=0644.

use std::path::{Component, Path, PathBuf};

/// OPENABLE are the extensions Open in default app opens: documents and
/// media, never anything a desktop runs or a browser scripts.
const OPENABLE: &[&str] = &[
    "md", "txt", "pdf", "png", "jpg", "jpeg", "gif", "webp", "bmp", "avif", "mp3", "wav", "ogg", "flac", "m4a",
    "webm", "mp4", "mov", "mkv", "ogv", "3gp", "canvas", "base", "csv", "odt", "ods", "odp", "doc", "docx", "xls",
    "xlsx", "ppt", "pptx", "rtf", "epub",
];

/// web_url says whether url may open in the system's browser.
pub fn web_url(url: &str) -> Result<tauri::Url, String> {
    let parsed = tauri::Url::parse(url).map_err(|e| format!("not a URL: {e}"))?;
    match parsed.scheme() {
        "http" | "https" if parsed.host_str().is_some_and(|h| !h.is_empty()) => Ok(parsed),
        "mailto" => Ok(mail_url(parsed)),
        other => Err(format!("{other}: links open outside the app only for http, https and mailto")),
    }
}

/// mail_url keeps a mailto link's addressees, subject and body, and drops
/// whatever else its query asks of the mail client, such as attaching a
/// file (attach=).
fn mail_url(mut url: tauri::Url) -> tauri::Url {
    let kept: Vec<String> = url
        .query()
        .unwrap_or("")
        .split('&')
        .filter(|part| {
            let key = part.split('=').next().unwrap_or("").to_ascii_lowercase();
            matches!(key.as_str(), "to" | "cc" | "bcc" | "subject" | "body")
        })
        .map(str::to_string)
        .collect();
    url.set_query((!kept.is_empty()).then(|| kept.join("&")).as_deref());
    url
}

/// vault_file resolves path inside the vault folder root to a file Open in
/// default app may open.
pub fn vault_file(root: &str, path: &str) -> Result<PathBuf, String> {
    let root = Path::new(root);
    if !root.is_absolute() {
        return Err("the vault's folder is not an absolute path".into());
    }
    let rel = Path::new(path);
    if path.is_empty() || !rel.components().all(|c| matches!(c, Component::Normal(_))) {
        return Err(format!("{path}: not a path inside the vault"));
    }
    let root = root.canonicalize().map_err(|e| format!("the vault's folder: {e}"))?;
    let file = root.join(rel).canonicalize().map_err(|e| format!("{path}: {e}"))?;
    if !file.starts_with(&root) {
        return Err(format!("{path} leads outside the vault"));
    }
    let meta = std::fs::metadata(&file).map_err(|e| format!("{path}: {e}"))?;
    if !meta.is_file() {
        return Err(format!("{path} is not a file"));
    }
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        if meta.permissions().mode() & 0o111 != 0 {
            return Err(format!("{path} is executable: the app does not run files"));
        }
    }
    let ext = file.extension().and_then(|e| e.to_str()).unwrap_or("").to_ascii_lowercase();
    if !OPENABLE.contains(&ext.as_str()) {
        return Err(format!("{path}: the app opens only documents and media outside itself"));
    }
    Ok(file)
}

fn xdg_open(arg: &std::ffi::OsStr) -> Result<(), String> {
    let mut child = std::process::Command::new("xdg-open")
        .arg(arg)
        .stdin(std::process::Stdio::null())
        .stdout(std::process::Stdio::null())
        .stderr(std::process::Stdio::null())
        .spawn()
        .map_err(|e| format!("xdg-open: {e}"))?;
    // Reaped when it exits, so no open leaves a zombie behind.
    std::thread::spawn(move || child.wait());
    Ok(())
}

/// open_url opens a web link in the system's browser.
#[tauri::command]
pub fn open_url(url: String) -> Result<(), String> {
    let url = web_url(&url)?;
    xdg_open(url.as_str().as_ref())
}

/// open_file opens a vault file in the system's default app.
#[tauri::command]
pub fn open_file(root: String, path: String) -> Result<(), String> {
    let file = vault_file(&root, &path)?;
    xdg_open(file.as_os_str())
}

#[cfg(test)]
mod tests {
    use super::{vault_file, web_url};
    use std::fs;

    #[test]
    fn opens_only_web_and_mail_links() {
        for ok in ["https://example.test/a?b=1", "http://example.test", "mailto:wren@example.test"] {
            assert!(web_url(ok).is_ok(), "{ok}");
        }
        for bad in ["file:///etc/passwd", "javascript:alert(1)", "notesblob://localhost/x", "smb://h/s", "x", "http://"] {
            assert!(web_url(bad).is_err(), "{bad}");
        }
    }

    // Review finding 7, plan 0005 Phase 7.
    #[test]
    fn mail_links_keep_only_addressees_subject_and_body() {
        let url = |s: &str| web_url(s).unwrap().to_string();
        assert_eq!(
            url("mailto:a@e.test?attach=/etc/passwd&subject=Hi%20there&Attachment=x&cc=b@e.test&ATTACH=y&body=a+b"),
            "mailto:a@e.test?subject=Hi%20there&cc=b@e.test&body=a+b"
        );
        assert_eq!(url("mailto:a@e.test?attach=/home/u/.ssh/id_ed25519"), "mailto:a@e.test");
        assert_eq!(url("mailto:a@e.test?%61ttach=x&To=c@e.test"), "mailto:a@e.test?To=c@e.test");
        assert_eq!(url("mailto:a@e.test"), "mailto:a@e.test");
    }

    #[test]
    fn opens_only_documents_inside_the_vault() {
        let dir = std::env::temp_dir().join(format!("frostnotes-open-{}", std::process::id()));
        let vault = dir.join("vault");
        fs::create_dir_all(vault.join("media")).unwrap();
        fs::write(vault.join("media/map.PNG"), b"png").unwrap();
        fs::write(vault.join("Plan.md"), b"# Plan").unwrap();
        fs::write(vault.join("run.desktop"), b"[Desktop Entry]").unwrap();
        fs::write(vault.join("page.html"), b"<script></script>").unwrap();
        fs::write(dir.join("outside.md"), b"x").unwrap();
        let root = vault.to_str().unwrap();

        assert!(vault_file(root, "media/map.PNG").is_ok());
        assert!(vault_file(root, "Plan.md").is_ok());
        for bad in ["run.desktop", "page.html", "../outside.md", "/etc/passwd", "", "media", "missing.pdf", "./Plan.md"] {
            assert!(vault_file(root, bad).is_err(), "{bad}");
        }
        assert!(vault_file("relative/root", "Plan.md").is_err());

        #[cfg(unix)]
        {
            use std::os::unix::fs::{PermissionsExt, symlink};
            fs::write(vault.join("tool.pdf"), b"x").unwrap();
            fs::set_permissions(vault.join("tool.pdf"), fs::Permissions::from_mode(0o755)).unwrap();
            assert!(vault_file(root, "tool.pdf").is_err());
            symlink(dir.join("outside.md"), vault.join("link.md")).unwrap();
            assert!(vault_file(root, "link.md").is_err());
        }
        fs::remove_dir_all(&dir).unwrap();
    }
}
