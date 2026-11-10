//! WebKitGTK evaluation and snapshots, answered through native completion
//! callbacks. The page is never handed a script message handler, so nothing it
//! runs can call back into the app.
use std::cell::RefCell;

use cairo::{Context, Format, ImageSurface};
use javascriptcore::ValueExt;
use tauri::{Runtime, Webview};
use tokio::sync::oneshot;
use webkit2gtk::gio::Cancellable;
use webkit2gtk::{
    NavigationPolicyDecision, NavigationPolicyDecisionExt, PolicyDecisionExt, PolicyDecisionType,
    SnapshotOptions, SnapshotRegion, URIRequestExt, WebViewExt,
};

use super::agent::Png;

fn once<T: 'static>(tx: oneshot::Sender<T>) -> impl Fn(T) {
    let slot = RefCell::new(Some(tx));
    move |value| {
        if let Some(tx) = slot.borrow_mut().take() {
            let _ = tx.send(value);
        }
    }
}

/// Runs `body` (an async function body) in the page's own world; the receiver
/// yields the string it returned.
pub fn call_async<R: Runtime>(
    webview: &Webview<R>,
    body: String,
) -> Result<oneshot::Receiver<Result<String, String>>, String> {
    let (tx, rx) = oneshot::channel();
    webview
        .with_webview(move |platform| {
            let reply = once(tx);
            // No world name: the default world, the page's own.
            platform.inner().call_async_javascript_function(
                &body,
                None,
                None,
                None,
                None::<&Cancellable>,
                move |result| {
                    reply(match result {
                        Ok(value) if value.is_string() => Ok(value.to_str().to_string()),
                        Ok(_) => Err("the script returned no value".to_string()),
                        Err(error) => Err(error.message().to_string()),
                    })
                },
            );
        })
        .map_err(|e| e.to_string())?;
    Ok(rx)
}

/// Snapshots the visible viewport, scaled down to `width` points wide when the
/// view is wider.
pub fn snapshot<R: Runtime>(
    webview: &Webview<R>,
    width: f64,
) -> Result<oneshot::Receiver<Result<Png, String>>, String> {
    let scale = webview.window().scale_factor().map_err(|e| e.to_string())?;
    let view_width = webview.size().map_err(|e| e.to_string())?.width as f64 / scale;
    let shrink = shrink_factor(view_width, width);
    let (tx, rx) = oneshot::channel();
    webview
        .with_webview(move |platform| {
            let reply = once(tx);
            platform.inner().snapshot(
                SnapshotRegion::Visible,
                SnapshotOptions::NONE,
                None::<&Cancellable>,
                move |result| {
                    reply(
                        result
                            .map_err(|e| e.message().to_string())
                            .and_then(|surface| encode(surface, shrink)),
                    )
                },
            );
        })
        .map_err(|e| e.to_string())?;
    Ok(rx)
}

/// How much of the view's width the snapshot keeps: never more than all of it.
fn shrink_factor(view_width: f64, wanted: f64) -> f64 {
    if view_width <= 0.0 {
        return 1.0;
    }
    (wanted / view_width).clamp(f64::MIN_POSITIVE, 1.0)
}

fn encode(surface: cairo::Surface, shrink: f64) -> Result<Png, String> {
    let source = ImageSurface::try_from(surface).map_err(|_| "the snapshot is not an image")?;
    let width = ((f64::from(source.width()) * shrink).round() as i32).max(1);
    let height = ((f64::from(source.height()) * shrink).round() as i32).max(1);
    let target = ImageSurface::create(Format::ARgb32, width, height).map_err(|e| e.to_string())?;
    let cx = Context::new(&target).map_err(|e| e.to_string())?;
    cx.scale(shrink, shrink);
    cx.set_source_surface(&source, 0.0, 0.0)
        .map_err(|e| e.to_string())?;
    cx.paint().map_err(|e| e.to_string())?;
    drop(cx);
    let mut bytes = Vec::new();
    target
        .write_to_png(&mut bytes)
        .map_err(|e| format!("the snapshot cannot be encoded as png: {e}"))?;
    Ok(Png {
        bytes,
        width: width as u32,
        height: height as u32,
    })
}

/// Hands `open` the url of every link that asks for a new window (target=_blank), and keeps
/// WebKit from making one. On GTK such a link never reaches Tauri's new-window handler; only
/// window.open does.
pub fn route_new_windows(view: &webkit2gtk::WebView, open: impl Fn(String) + 'static) {
    use gtk::prelude::Cast;
    view.connect_decide_policy(move |_, decision, kind| {
        if kind != PolicyDecisionType::NewWindowAction {
            return false;
        }
        let url = decision
            .downcast_ref::<NavigationPolicyDecision>()
            .and_then(|d| d.navigation_action())
            .and_then(|action| action.request())
            .and_then(|request| request.uri());
        let Some(url) = url else {
            return false;
        };
        open(url.to_string());
        decision.ignore();
        true
    });
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::browser::gtk_test_thread::on_gtk_thread;
    use std::rc::Rc;
    use std::time::{Duration, Instant};
    use webkit2gtk::LoadEvent;

    /// Runs GTK until `done`, for at most a few seconds; reports whether it got there.
    fn run_until(done: impl Fn() -> bool) -> bool {
        let deadline = Instant::now() + Duration::from_secs(10);
        while !done() && Instant::now() < deadline {
            gtk::main_iteration_do(false);
        }
        done()
    }

    /// Runs `script` in a page with a target=_blank link; reports the urls handed to
    /// `route_new_windows` and the WebKit signals seen. `create` is where Tauri's own
    /// new-window handler would be asked.
    fn new_windows_from(script: &'static str) -> (Vec<String>, Vec<String>) {
        on_gtk_thread(move || {
            let view = webkit2gtk::WebView::new();
            let opened = Rc::new(RefCell::new(Vec::new()));
            let seen = Rc::new(RefCell::new(Vec::new()));
            let into = opened.clone();
            route_new_windows(&view, move |url| into.borrow_mut().push(url));
            let log = seen.clone();
            view.connect_decide_policy(move |_, _, kind| {
                log.borrow_mut().push(format!("policy {kind:?}"));
                false
            });
            let log = seen.clone();
            view.connect_create(move |_, _| {
                log.borrow_mut().push("create".into());
                None
            });
            let loaded = Rc::new(RefCell::new(false));
            let flag = loaded.clone();
            view.connect_load_changed(move |_, event| {
                if event == LoadEvent::Finished {
                    *flag.borrow_mut() = true;
                }
            });
            let html = "<a id=a href='https://example.test/next' target=_blank>next</a>";
            view.load_html(html, Some("https://example.test/"));
            assert!(run_until(|| *loaded.borrow()), "the page loads");
            view.call_async_javascript_function(
                script,
                None,
                None,
                None,
                None::<&Cancellable>,
                |_| {},
            );
            let asked =
                || !opened.borrow().is_empty() || seen.borrow().iter().any(|s| s == "create");
            run_until(asked);
            let opened = opened.borrow().clone();
            let seen = seen.borrow().clone();
            (opened, seen)
        })
    }

    #[test]
    fn a_target_blank_link_asks_for_one_new_window() {
        let (opened, seen) = new_windows_from("document.getElementById('a').click()");
        assert_eq!(opened, ["https://example.test/next"], "signals: {seen:?}");
        assert!(!seen.iter().any(|s| s == "create"), "no window: {seen:?}");
    }

    #[test]
    fn window_open_asks_for_one_window_either_way() {
        let (opened, seen) = new_windows_from("window.open('https://example.test/popup')");
        let created = seen.iter().filter(|s| *s == "create").count();
        assert_eq!(
            opened.len() + created,
            1,
            "routed {opened:?}, signals {seen:?}"
        );
    }

    #[test]
    fn a_snapshot_never_grows_and_keeps_the_requested_width() {
        assert_eq!(shrink_factor(800.0, 1600.0), 1.0);
        assert_eq!(shrink_factor(2000.0, 1000.0), 0.5);
        assert_eq!(shrink_factor(0.0, 500.0), 1.0);
    }
}
