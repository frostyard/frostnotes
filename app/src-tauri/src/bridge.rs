//! A line bridge between the webview and notesd's Unix socket. It never
//! parses frames: JSON-RPC multiplexing happens in the webview
//! (src/rpc/transport.ts), so the bridge cannot drift from the API
//! (docs/specs/rpc-protocol.md).

use std::collections::HashMap;

use serde::Serialize;
use tauri::{State, Window, ipc::Channel};
use tokio::io::{AsyncBufReadExt, AsyncWriteExt, BufReader};
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
        let mut lines = BufReader::new(read).lines();
        let error = loop {
            match lines.next_line().await {
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
    use super::frame;

    #[test]
    fn frames_are_one_line() {
        assert_eq!(frame(r#"{"jsonrpc":"2.0"}"#.into()), Ok(b"{\"jsonrpc\":\"2.0\"}\n".to_vec()));
        assert!(frame("{}\n{}".into()).is_err());
    }
}
