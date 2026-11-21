//! Keeps the caret blink of the shell's own fields on the desktop's, as the web views report it.

use std::sync::Arc;

use gpui::{App, AsyncApp};

pub(crate) type CaretChanges = async_channel::Receiver<webview::CaretBlink>;

pub(crate) fn channel() -> (webview::CaretReport, CaretChanges) {
    let (sender, changes) = async_channel::unbounded();
    let report: webview::CaretReport = Arc::new(move |blink| {
        sender.try_send(blink).unwrap_or_default();
    });
    (report, changes)
}

pub(crate) fn follow(
    changes: CaretChanges,
    cx: &mut App,
) {
    cx.spawn(async move |cx| relay(&changes, cx).await).detach();
}

async fn relay(
    changes: &CaretChanges,
    cx: &mut AsyncApp,
) {
    while let Ok(blink) = changes.recv().await {
        cx.update(|cx| apply(blink, cx));
    }
}

fn apply(
    blink: webview::CaretBlink,
    cx: &mut App,
) {
    cx.set_global(ui_blink(blink));
    cx.refresh_windows();
}

fn ui_blink(blink: webview::CaretBlink) -> ui::CaretBlink {
    match blink {
        webview::CaretBlink::Steady => ui::CaretBlink::Steady,
        webview::CaretBlink::Every(phase) => ui::CaretBlink::Every(phase),
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use gpui::TestAppContext;

    use super::*;

    #[gpui::test]
    fn the_shell_caret_takes_each_blink_the_desktop_reports(cx: &mut TestAppContext) {
        let (report, changes) = channel();
        cx.update(|cx| follow(changes, cx));
        report(webview::CaretBlink::Steady);
        cx.run_until_parked();
        assert_eq!(
            cx.update(|cx| ui::CaretBlink::of(cx)),
            ui::CaretBlink::Steady
        );
        let phase = Duration::from_millis(400);
        report(webview::CaretBlink::Every(phase));
        cx.run_until_parked();
        assert_eq!(
            cx.update(|cx| ui::CaretBlink::of(cx)),
            ui::CaretBlink::Every(phase)
        );
    }
}
