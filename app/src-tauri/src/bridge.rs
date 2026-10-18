//! A line bridge between the webview and notesd's Unix socket. It never
//! parses frames: JSON-RPC multiplexing happens in the webview
//! (src/rpc/transport.ts), so the bridge cannot drift from the API
//! (docs/specs/rpc-protocol.md).

use std::collections::HashMap;

use serde::Serialize;
use tauri::{State, Window, ipc::Channel};
use tokio::io::{AsyncBufRead, AsyncBufReadExt, AsyncReadExt, AsyncWriteExt, BufReader};
use tokio::net::UnixStream;
use tokio::net::unix::OwnedWriteHalf;
use tokio::sync::Mutex;

use crate::paths;

/// What the webview receives on its channel, in order.
#[derive(Clone, Serialize)]
#[serde(tag = "kind", rename_all = "camelCase")]
pub enum BridgeMessage {
    /// One newline-delimited JSON-RPC message from notesd.
    Line { line: String },
    /// The connection ended; no more lines follow.
    Closed { error: String },
}

/// Each window's connection write half, by window label: every window has
/// its own connection. Connecting again replaces the window's connection,
/// which shuts the old one down; closing the window drops it.
#[derive(Default)]
pub struct Bridge {
    writers: Mutex<HashMap<String, OwnedWriteHalf>>,
}

impl Bridge {
    /// Drop a closed window's connection.
    pub async fn forget(&self, label: &str) {
        self.writers.lock().await.remove(label);
    }
}

/// Connect the calling window to notesd and stream every received line to
/// `on_message`.
#[tauri::command]
pub async fn notesd_connect(
    window: Window,
    bridge: State<'_, Bridge>,
    on_message: Channel<BridgeMessage>,
) -> Result<(), String> {
    let path = paths::socket()?;
    let stream = UnixStream::connect(&path)
        .await
        .map_err(|e| format!("connect to notesd at {}: {e}", path.display()))?;
    let (read, write) = stream.into_split();
    bridge.writers.lock().await.insert(window.label().to_string(), write);
    tauri::async_runtime::spawn(async move {
        let mut reader = BufReader::new(read);
        let error = loop {
            match read_line(&mut reader, MAX_LINE).await {
                Ok(Some(line)) => {
                    if on_message.send(BridgeMessage::Line { line }).is_err() {
                        break "the webview stopped listening".to_string();
                    }
                }
                Ok(None) => break "notesd closed the connection".to_string(),
                Err(e) => break format!("read from notesd: {e}"),
            }
        };
        let _ = on_message.send(BridgeMessage::Closed { error });
    });
    Ok(())
}

/// Send one JSON-RPC message on the calling window's connection; it must not
/// contain a newline.
#[tauri::command]
pub async fn notesd_send(window: Window, bridge: State<'_, Bridge>, line: String) -> Result<(), String> {
    let frame = frame(line)?;
    let mut guard = bridge.writers.lock().await;
    let writer = guard.get_mut(window.label()).ok_or("not connected to notesd")?;
    writer.write_all(&frame).await.map_err(|e| format!("write to notesd: {e}"))
}

/// MAX_LINE is the longest message notesd sends (api.MaxMessageSize in
/// docs/specs/rpc-protocol.md): a longer line ends the connection rather
/// than fill the shell's memory.
const MAX_LINE: usize = 16 << 20;

/// read_line reads one line, without its newline, of at most max bytes;
/// None at the end of the input.
async fn read_line<R: AsyncBufRead + Unpin>(r: &mut R, max: usize) -> std::io::Result<Option<String>> {
    let mut buf = Vec::new();
    if (&mut *r).take(max as u64 + 1).read_until(b'\n', &mut buf).await? == 0 {
        return Ok(None);
    }
    if buf.last() == Some(&b'\n') {
        buf.pop();
    } else if buf.len() > max {
        return Err(std::io::Error::new(std::io::ErrorKind::InvalidData, format!("a line from notesd is over {max} bytes")));
    }
    String::from_utf8(buf).map(Some).map_err(|e| std::io::Error::new(std::io::ErrorKind::InvalidData, e))
}

/// The bytes on the wire for one message: the line and its newline.
fn frame(line: String) -> Result<Vec<u8>, String> {
    if line.contains('\n') {
        return Err("a frame must be a single line".into());
    }
    let mut frame = line.into_bytes();
    frame.push(b'\n');
    Ok(frame)
}

#[cfg(test)]
mod tests {
    use super::{frame, read_line};

    #[test]
    fn reads_lines_no_longer_than_a_message() {
        tauri::async_runtime::block_on(async {
            let mut input: &[u8] = b"abcd\nef\ngh";
            assert_eq!(read_line(&mut input, 4).await.unwrap().as_deref(), Some("abcd"));
            assert_eq!(read_line(&mut input, 4).await.unwrap().as_deref(), Some("ef"));
            assert_eq!(read_line(&mut input, 4).await.unwrap().as_deref(), Some("gh"));
            assert_eq!(read_line(&mut input, 4).await.unwrap(), None);
            let mut long: &[u8] = b"abcde\nf\n";
            assert!(read_line(&mut long, 4).await.is_err());
        });
    }

    #[test]
    fn frames_are_one_line() {
        assert_eq!(frame(r#"{"jsonrpc":"2.0"}"#.into()), Ok(b"{\"jsonrpc\":\"2.0\"}\n".to_vec()));
        assert!(frame("{}\n{}".into()).is_err());
    }
}
