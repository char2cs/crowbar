//! WKWebView evaluation and snapshots, answered through native completion
//! handlers. The page is never handed a message handler, so nothing it runs can
//! call back into the app.
use block2::RcBlock;
use objc2::runtime::AnyObject;
use objc2::MainThreadMarker;
use objc2_app_kit::{NSBitmapImageFileType, NSBitmapImageRep, NSImage};
use objc2_foundation::{NSDictionary, NSError, NSNumber, NSString};
use objc2_web_kit::{WKContentWorld, WKSnapshotConfiguration, WKWebView};
use std::cell::RefCell;
use tauri::{Runtime, Webview};
use tokio::sync::oneshot;

use super::agent::Png;

const JS_EXCEPTION_KEY: &str = "WKJavaScriptExceptionMessage";

fn once<T: 'static>(tx: oneshot::Sender<T>) -> impl Fn(T) {
    let slot = RefCell::new(Some(tx));
    move |value| {
        if let Some(tx) = slot.borrow_mut().take() {
            let _ = tx.send(value);
        }
    }
}

fn describe(error: &NSError) -> String {
    let detail = error
        .userInfo()
        .objectForKey(&NSString::from_str(JS_EXCEPTION_KEY))
        .and_then(|v| v.downcast_ref::<NSString>().map(|s| s.to_string()));
    detail.unwrap_or_else(|| error.localizedDescription().to_string())
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
            // SAFETY: `inner` is the live WKWebView, and this closure runs on the
            // main thread, where the WebKit calls below are allowed.
            unsafe {
                let wk: &WKWebView = &*platform.inner().cast();
                let Some(mtm) = MainThreadMarker::new() else {
                    return reply(Err("not on the main thread".into()));
                };
                let handler = RcBlock::new(move |result: *mut AnyObject, error: *mut NSError| {
                    if let Some(error) = error.as_ref() {
                        return reply(Err(describe(error)));
                    }
                    let text = result
                        .as_ref()
                        .and_then(|v| v.downcast_ref::<NSString>())
                        .map(|s| s.to_string());
                    reply(text.ok_or_else(|| "the script returned no value".to_string()));
                });
                wk.callAsyncJavaScript_arguments_inFrame_inContentWorld_completionHandler(
                    &NSString::from_str(&body),
                    None,
                    None,
                    &WKContentWorld::pageWorld(mtm),
                    Some(&handler),
                );
            }
        })
        .map_err(|e| e.to_string())?;
    Ok(rx)
}

fn encode(image: &NSImage) -> Result<Png, String> {
    let tiff = image
        .TIFFRepresentation()
        .ok_or("the snapshot has no pixel data")?;
    let rep = NSBitmapImageRep::imageRepWithData(&tiff).ok_or("the snapshot cannot be read")?;
    let png = unsafe {
        rep.representationUsingType_properties(NSBitmapImageFileType::PNG, &NSDictionary::new())
    }
    .ok_or("the snapshot cannot be encoded as png")?;
    Ok(Png {
        bytes: png.to_vec(),
        width: rep.pixelsWide().max(0) as u32,
        height: rep.pixelsHigh().max(0) as u32,
    })
}

/// Snapshots the visible viewport at `width` points wide.
pub fn snapshot<R: Runtime>(
    webview: &Webview<R>,
    width: f64,
) -> Result<oneshot::Receiver<Result<Png, String>>, String> {
    let (tx, rx) = oneshot::channel();
    webview
        .with_webview(move |platform| {
            let reply = once(tx);
            // SAFETY: as in `call_async`.
            unsafe {
                let wk: &WKWebView = &*platform.inner().cast();
                let Some(mtm) = MainThreadMarker::new() else {
                    return reply(Err("not on the main thread".into()));
                };
                let config = WKSnapshotConfiguration::new(mtm);
                config.setSnapshotWidth(Some(&NSNumber::new_f64(width)));
                let handler = RcBlock::new(move |image: *mut NSImage, error: *mut NSError| {
                    if let Some(error) = error.as_ref() {
                        return reply(Err(describe(error)));
                    }
                    match image.as_ref() {
                        Some(image) => reply(encode(image)),
                        None => reply(Err("the snapshot is empty".into())),
                    }
                });
                wk.takeSnapshotWithConfiguration_completionHandler(Some(&config), &handler);
            }
        })
        .map_err(|e| e.to_string())?;
    Ok(rx)
}
