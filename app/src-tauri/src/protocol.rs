//! notesblob://localhost/<sha256>.<ext> serves notesd's blob store by
//! content hash, so the webview shows a vault's images, audio and video
//! without their bytes crossing JSON-RPC (ADR-0023). It takes no path, only
//! a hash, so nothing outside the blob store can be named; the extension
//! only chooses the content type. The dev gateway's /blobs/ serves the same
//! in a browser (internal/devgw).

use std::fs::File;
use std::io::{Read, Seek, SeekFrom};
use std::path::{Path, PathBuf};

use tauri::http::{Request, Response, StatusCode, header};
use tauri::{Runtime, UriSchemeContext, UriSchemeResponder};

use crate::paths;

pub fn notesblob<R: Runtime>(_ctx: UriSchemeContext<'_, R>, request: Request<Vec<u8>>, responder: UriSchemeResponder) {
    let path = request.uri().path().to_string();
    let range = request.headers().get(header::RANGE).and_then(|v| v.to_str().ok()).map(str::to_string);
    tauri::async_runtime::spawn_blocking(move || {
        let blobs = paths::data_dir().map(|d| d.join("blobs"));
        let response = match blobs {
            Ok(dir) => serve(&dir, &path, range.as_deref()),
            Err(_) => status(StatusCode::INTERNAL_SERVER_ERROR),
        };
        responder.respond(response)
    });
}

/// parse returns a URL path's hash and extension: "/", 64 lowercase hex
/// digits, ".", and 1 to 8 lowercase letters or digits.
pub fn parse(path: &str) -> Option<(&str, &str)> {
    let rest = path.strip_prefix('/')?;
    let (hash, ext) = rest.split_once('.')?;
    let hex = hash.len() == 64 && hash.bytes().all(|b| b.is_ascii_digit() || (b'a'..=b'f').contains(&b));
    let ext_ok = (1..=8).contains(&ext.len()) && ext.bytes().all(|b| b.is_ascii_lowercase() || b.is_ascii_digit());
    (hex && ext_ok).then_some((hash, ext))
}

/// content_type is the media type of a vault file by its extension; the
/// table is internal/devgw's ContentType (docs/design/app.md#attachments).
pub fn content_type(ext: &str) -> &'static str {
    match ext {
        "png" => "image/png",
        "jpg" | "jpeg" => "image/jpeg",
        "gif" => "image/gif",
        "webp" => "image/webp",
        "avif" => "image/avif",
        "bmp" => "image/bmp",
        "svg" => "image/svg+xml",
        "mp3" => "audio/mpeg",
        "wav" => "audio/wav",
        "m4a" => "audio/mp4",
        "3gp" => "audio/3gpp",
        "flac" => "audio/flac",
        "ogg" | "oga" => "audio/ogg",
        "opus" => "audio/opus",
        "mp4" => "video/mp4",
        "webm" => "video/webm",
        "ogv" => "video/ogg",
        "mov" => "video/quicktime",
        "mkv" => "video/x-matroska",
        "pdf" => "application/pdf",
        "md" => "text/markdown; charset=utf-8",
        _ => "application/octet-stream",
    }
}

/// parse_range reads one byte range of a file of len bytes: "bytes=a-b",
/// "bytes=a-" or "bytes=-n", as inclusive offsets; None when it is not one
/// range or not satisfiable.
pub fn parse_range(value: &str, len: u64) -> Option<(u64, u64)> {
    let spec = value.strip_prefix("bytes=")?;
    if spec.contains(',') || len == 0 {
        return None;
    }
    let (a, b) = spec.split_once('-')?;
    let (start, end) = match (a.trim(), b.trim()) {
        ("", n) => {
            let n: u64 = n.parse().ok()?;
            (len.saturating_sub(n), len - 1)
        }
        (a, "") => (a.parse().ok()?, len - 1),
        (a, b) => (a.parse().ok()?, b.parse::<u64>().ok()?.min(len - 1)),
    };
    (start <= end && start < len).then_some((start, end))
}

fn status(code: StatusCode) -> Response<Vec<u8>> {
    let mut r = Response::new(Vec::new());
    *r.status_mut() = code;
    r
}

/// WINDOW is the most bytes one 206 answer carries: a media element reads
/// on with further ranges, so a large video never sits whole in memory.
const WINDOW: u64 = 8 << 20;

/// WHOLE is the largest blob answered whole, to a request without a range
/// (an image, a document); a larger one must be asked for in ranges.
const WHOLE: u64 = 64 << 20;

/// serve answers a request for path from the blob store at blobs.
pub fn serve(blobs: &Path, path: &str, range: Option<&str>) -> Response<Vec<u8>> {
    serve_within(blobs, path, range, WINDOW, WHOLE)
}

/// serve_within is serve with the window and the whole-file limit given.
fn serve_within(blobs: &Path, path: &str, range: Option<&str>, window: u64, whole: u64) -> Response<Vec<u8>> {
    let Some((hash, ext)) = parse(path) else {
        return status(StatusCode::NOT_FOUND);
    };
    let file: PathBuf = blobs.join(&hash[..2]).join(hash);
    let Ok(mut f) = File::open(&file) else {
        return status(StatusCode::NOT_FOUND);
    };
    let Ok(len) = f.metadata().map(|m| m.len()) else {
        return status(StatusCode::INTERNAL_SERVER_ERROR);
    };
    let builder = Response::builder()
        .header(header::CONTENT_TYPE, content_type(ext))
        .header("X-Content-Type-Options", "nosniff")
        // A file opened by its URL is passive content: an SVG must not run
        // script or load anything.
        .header("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; sandbox")
        .header(header::CACHE_CONTROL, "private, max-age=31536000, immutable")
        .header(header::ACCEPT_RANGES, "bytes");
    let result = match range {
        None if len > whole => {
            return Response::builder()
                .status(StatusCode::PAYLOAD_TOO_LARGE)
                .header(header::ACCEPT_RANGES, "bytes")
                .body(Vec::new())
                .unwrap_or_else(|_| status(StatusCode::PAYLOAD_TOO_LARGE));
        }
        None => {
            let mut body = Vec::with_capacity(len as usize);
            f.read_to_end(&mut body).map(|_| builder.status(StatusCode::OK).body(body))
        }
        Some(value) => match parse_range(value, len) {
            None => {
                return Response::builder()
                    .status(StatusCode::RANGE_NOT_SATISFIABLE)
                    .header(header::CONTENT_RANGE, format!("bytes */{len}"))
                    .body(Vec::new())
                    .unwrap_or_else(|_| status(StatusCode::RANGE_NOT_SATISFIABLE));
            }
            Some((start, end)) => {
                let end = end.min(start + window - 1);
                let mut body = vec![0; (end - start + 1) as usize];
                f.seek(SeekFrom::Start(start)).and_then(|_| f.read_exact(&mut body)).map(|_| {
                    builder
                        .status(StatusCode::PARTIAL_CONTENT)
                        .header(header::CONTENT_RANGE, format!("bytes {start}-{end}/{len}"))
                        .body(body)
                })
            }
        },
    };
    match result {
        Ok(Ok(response)) => response,
        _ => status(StatusCode::INTERNAL_SERVER_ERROR),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const HASH: &str = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef";

    fn store(content: &[u8]) -> PathBuf {
        let dir = std::env::temp_dir().join(format!("notesblob-test-{}-{}", std::process::id(), content.len()));
        let shard = dir.join(&HASH[..2]);
        std::fs::create_dir_all(&shard).unwrap();
        std::fs::write(shard.join(HASH), content).unwrap();
        dir
    }

    #[test]
    fn parses_only_a_hash_and_an_extension() {
        assert_eq!(parse(&format!("/{HASH}.png")), Some((HASH, "png")));
        assert_eq!(parse(&format!("/{HASH}.3gp")), Some((HASH, "3gp")));
        for bad in [
            format!("{HASH}.png"),
            format!("/{HASH}"),
            format!("/{HASH}."),
            format!("/{}.png", HASH.to_uppercase()),
            format!("/{}.png", &HASH[..63]),
            format!("/{HASH}.PNG"),
            format!("/{HASH}.png/x"),
            format!("/{HASH}.toolongext"),
            "/../notesd.db".to_string(),
            "/".to_string(),
        ] {
            assert_eq!(parse(&bad), None, "{bad}");
        }
    }

    #[test]
    fn names_content_types_as_the_dev_gateway_does() {
        assert_eq!(content_type("png"), "image/png");
        assert_eq!(content_type("jpeg"), "image/jpeg");
        assert_eq!(content_type("svg"), "image/svg+xml");
        assert_eq!(content_type("oga"), "audio/ogg");
        assert_eq!(content_type("mkv"), "video/x-matroska");
        assert_eq!(content_type("md"), "text/markdown; charset=utf-8");
        assert_eq!(content_type("exe"), "application/octet-stream");
    }

    #[test]
    fn reads_one_range() {
        assert_eq!(parse_range("bytes=0-9", 100), Some((0, 9)));
        assert_eq!(parse_range("bytes=90-", 100), Some((90, 99)));
        assert_eq!(parse_range("bytes=-10", 100), Some((90, 99)));
        assert_eq!(parse_range("bytes=90-200", 100), Some((90, 99)));
        assert_eq!(parse_range("bytes=-200", 100), Some((0, 99)));
        for bad in ["bytes=100-", "bytes=9-0", "bytes=0-1,4-5", "items=0-1", "bytes=a-b", "bytes=-"] {
            assert_eq!(parse_range(bad, 100), None, "{bad}");
        }
        assert_eq!(parse_range("bytes=0-", 0), None);
    }

    #[test]
    fn serves_a_blob_whole_and_in_part() {
        let content = b"not really a PNG, but bytes all the same\n";
        let dir = store(content);
        let path = format!("/{HASH}.png");

        let r = serve(&dir, &path, None);
        assert_eq!(r.status(), StatusCode::OK);
        assert_eq!(r.body().as_slice(), content);
        let h = r.headers();
        assert_eq!(h[header::CONTENT_TYPE], "image/png");
        assert_eq!(h["X-Content-Type-Options"], "nosniff");
        assert_eq!(h["Content-Security-Policy"], "default-src 'none'; style-src 'unsafe-inline'; sandbox");
        assert_eq!(h[header::CACHE_CONTROL], "private, max-age=31536000, immutable");

        let r = serve(&dir, &format!("/{HASH}.mp3"), Some("bytes=4-9"));
        assert_eq!(r.status(), StatusCode::PARTIAL_CONTENT);
        assert_eq!(r.body().as_slice(), &content[4..10]);
        assert_eq!(r.headers()[header::CONTENT_RANGE], format!("bytes 4-9/{}", content.len()));
        assert_eq!(r.headers()[header::CONTENT_TYPE], "audio/mpeg");

        let r = serve(&dir, &path, Some("bytes=1000-"));
        assert_eq!(r.status(), StatusCode::RANGE_NOT_SATISFIABLE);
        assert_eq!(r.headers()[header::CONTENT_RANGE], format!("bytes */{}", content.len()));

        assert_eq!(serve(&dir, &format!("/{}.png", "f".repeat(64)), None).status(), StatusCode::NOT_FOUND);
        assert_eq!(serve(&dir, "/../notesd.db", None).status(), StatusCode::NOT_FOUND);
        std::fs::remove_dir_all(dir).unwrap();
    }

    // A large blob is never read whole (review finding 3, plan 0005 Phase 7).
    #[test]
    fn serves_a_large_blob_a_window_at_a_time() {
        let content: Vec<u8> = (0..100u8).collect();
        let dir = store(&content);
        let path = format!("/{HASH}.mp4");

        for (range, start, end) in [("bytes=0-", 0, 15), ("bytes=90-", 90, 99), ("bytes=10-50", 10, 25), ("bytes=-30", 70, 85)] {
            let r = serve_within(&dir, &path, Some(range), 16, 64);
            assert_eq!(r.status(), StatusCode::PARTIAL_CONTENT, "{range}");
            assert_eq!(r.body().as_slice(), &content[start..=end], "{range}");
            assert_eq!(r.headers()[header::CONTENT_RANGE], format!("bytes {start}-{end}/100"), "{range}");
        }
        let r = serve_within(&dir, &path, None, 16, 64);
        assert_eq!(r.status(), StatusCode::PAYLOAD_TOO_LARGE);
        assert!(r.body().is_empty());
        assert_eq!(serve_within(&dir, &path, None, 16, 100).status(), StatusCode::OK);
        std::fs::remove_dir_all(dir).unwrap();
    }
}
