//! Evaluation and screenshots for agent-driven browser tabs. Results come back
//! through native completion handlers (see `webkit`), never through a channel the
//! page could post to, so a remote page still has no way to call into the app.
use std::time::Duration;

use tauri::{AppHandle, Runtime};

use super::find;
#[cfg(target_os = "macos")]
use super::webkit;

/// Longest script a command accepts.
const MAX_BODY_BYTES: usize = 1 << 20;
/// Longest a single evaluation may run, however long the caller asks for.
const MAX_TIMEOUT_MS: u64 = 60_000;
/// Slack past the caller's own deadline, so a page-side timeout reports first.
const GRACE_MS: u64 = 2_000;
/// Widest snapshot, in points; a retina screen doubles it in pixels.
const MAX_SNAPSHOT_WIDTH: f64 = 1600.0;
const DEFAULT_SNAPSHOT_WIDTH: f64 = 1280.0;

pub struct Png {
    pub bytes: Vec<u8>,
    pub width: u32,
    pub height: u32,
}

#[cfg(not(target_os = "macos"))]
mod webkit {
    use super::Png;
    use tauri::{Runtime, Webview};
    use tokio::sync::oneshot;

    const UNSUPPORTED: &str = "agent browser control is only supported on macOS";

    pub fn call_async<R: Runtime>(
        _: &Webview<R>,
        _: String,
    ) -> Result<oneshot::Receiver<Result<String, String>>, String> {
        Err(UNSUPPORTED.into())
    }

    pub fn snapshot<R: Runtime>(
        _: &Webview<R>,
        _: f64,
    ) -> Result<oneshot::Receiver<Result<Png, String>>, String> {
        Err(UNSUPPORTED.into())
    }
}

/// The script as an async function body whose value always comes back as JSON.
fn wrap_body(body: &str) -> String {
    format!(
        "const __value = await (async () => {{\n{body}\n}})();\n\
         return JSON.stringify(__value === undefined ? null : __value);"
    )
}

fn clamp_timeout(ms: u64) -> Duration {
    Duration::from_millis(ms.clamp(1, MAX_TIMEOUT_MS) + GRACE_MS)
}

fn snapshot_width(view_width: f64, requested: Option<f64>) -> f64 {
    let wanted = requested.unwrap_or(DEFAULT_SNAPSHOT_WIDTH);
    wanted.min(view_width).clamp(1.0, MAX_SNAPSHOT_WIDTH)
}

/// Runs `body` (an async function body) in tab `id`; returns its JSON-encoded value.
#[tauri::command]
pub async fn browser_eval<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    body: String,
    timeout_ms: u64,
) -> Result<String, String> {
    if body.len() > MAX_BODY_BYTES {
        return Err(format!("script is larger than {MAX_BODY_BYTES} bytes"));
    }
    let webview = find(&app, &id)?;
    let reply = webkit::call_async(&webview, wrap_body(&body))?;
    let deadline = clamp_timeout(timeout_ms);
    match tokio::time::timeout(deadline, reply).await {
        Ok(Ok(result)) => result,
        Ok(Err(_)) => Err("the page went away before answering".into()),
        Err(_) => Err("the page did not answer in time".into()),
    }
}

/// PNG of the tab's visible viewport, at most `max_width` points wide.
#[tauri::command]
pub async fn browser_screenshot<R: Runtime>(
    app: AppHandle<R>,
    id: String,
    max_width: Option<f64>,
) -> Result<serde_json::Value, String> {
    use base64::Engine;
    let webview = find(&app, &id)?;
    let scale = webview.window().scale_factor().map_err(|e| e.to_string())?;
    let view_width = webview.size().map_err(|e| e.to_string())?.width as f64 / scale;
    let reply = webkit::snapshot(&webview, snapshot_width(view_width, max_width))?;
    let png = match tokio::time::timeout(clamp_timeout(10_000), reply).await {
        Ok(Ok(result)) => result?,
        Ok(Err(_)) => return Err("the page went away before the snapshot".into()),
        Err(_) => return Err("the snapshot did not finish in time".into()),
    };
    Ok(serde_json::json!({
        "png": base64::engine::general_purpose::STANDARD.encode(&png.bytes),
        "width": png.width,
        "height": png.height,
    }))
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_script_runs_as_an_async_body_and_answers_json() {
        let wrapped = wrap_body("return 1 + 1");
        assert!(wrapped.contains("await (async () => {\nreturn 1 + 1\n})()"));
        assert!(wrapped.ends_with("return JSON.stringify(__value === undefined ? null : __value);"));
    }

    #[test]
    fn evaluation_deadlines_are_bounded_with_room_for_the_page_to_report_first() {
        assert_eq!(clamp_timeout(0), Duration::from_millis(1 + GRACE_MS));
        assert_eq!(
            clamp_timeout(5_000),
            Duration::from_millis(5_000 + GRACE_MS)
        );
        assert_eq!(
            clamp_timeout(u64::MAX),
            Duration::from_millis(MAX_TIMEOUT_MS + GRACE_MS)
        );
    }

    #[test]
    fn snapshots_never_exceed_the_cap_or_the_view() {
        assert_eq!(snapshot_width(900.0, None), 900.0);
        assert_eq!(snapshot_width(3000.0, None), DEFAULT_SNAPSHOT_WIDTH);
        assert_eq!(snapshot_width(3000.0, Some(99_999.0)), MAX_SNAPSHOT_WIDTH);
        assert_eq!(snapshot_width(3000.0, Some(-5.0)), 1.0);
        assert_eq!(snapshot_width(500.0, Some(800.0)), 500.0);
    }
}
