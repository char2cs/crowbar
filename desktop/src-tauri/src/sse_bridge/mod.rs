use bytes::Bytes;
use http_body_util::{BodyExt, Empty};
use hyper_util::client::legacy::Client;
use hyper_util::rt::TokioExecutor;
use hyperlocal::{UnixConnector, Uri as UnixUri};
use std::time::Duration;
use tauri::WebviewWindow;

use crate::sidecar::socket_path;

pub async fn start(window: WebviewWindow) {
    if let Err(e) = run(&window).await {
        log::error!("SSE bridge error: {e}");
    }
}

async fn run(window: &WebviewWindow) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
    // Wait for the socket to be ready (sidecar may still be starting)
    let socket = socket_path();
    for _ in 0..50 {
        if socket.exists() {
            break;
        }
        tokio::time::sleep(Duration::from_millis(200)).await;
    }

    let url: hyper::Uri = UnixUri::new(&socket, "/api/v0/events").into();
    let client: Client<UnixConnector, Empty<Bytes>> =
        Client::builder(TokioExecutor::new()).build(UnixConnector);

    let resp = client.get(url).await?;
    let mut body = resp.into_body();

    let mut line_buf = String::new();
    let mut event_type = String::new();
    let mut event_data = String::new();

    while let Some(frame) = body.frame().await {
        let frame = frame?;
        if let Ok(chunk) = frame.into_data() {
            let text = String::from_utf8_lossy(&chunk);
            line_buf.push_str(&text);

            // Process complete lines
            while let Some(pos) = line_buf.find('\n') {
                let line = line_buf[..pos].trim_end_matches('\r').to_string();
                line_buf.drain(..=pos);

                if line.is_empty() {
                    // Blank line = end of event — dispatch if we have a type
                    if !event_type.is_empty() {
                        let js = format!(
                            "window.__crowbarPushEvent({}, {})",
                            serde_json::to_string(&event_type)?,
                            serde_json::to_string(&event_data)?,
                        );
                        let _ = window.eval(&js);
                    }
                    event_type.clear();
                    event_data.clear();
                } else if let Some(val) = line.strip_prefix("event:") {
                    event_type = val.trim().to_string();
                } else if let Some(val) = line.strip_prefix("data:") {
                    event_data = val.trim().to_string();
                }
                // ignore other SSE fields (id:, retry:, comments)
            }
        }
    }

    Ok(())
}
