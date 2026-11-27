use std::thread;

use gpui::{App, AsyncApp, Window, WindowId};
use platform::{FocusChange, InputFocus};

const THREAD: &str = "crowbar-window-focus";

pub(crate) fn follow(
    window: &Window,
    cx: &mut App,
) {
    if let Err(error) = start(window, cx) {
        tracing::warn!(%error, "the window's keyboard focus cannot be followed");
    }
}

fn start(
    window: &Window,
    cx: &mut App,
) -> anyhow::Result<()> {
    let webview::ParentWindow::X11(xid) = webview::ParentWindow::from_window(window)? else {
        return Ok(());
    };
    let focus = InputFocus::connect()?;
    focus.follow(xid)?;
    let (changes, heard) = async_channel::unbounded();
    thread::Builder::new()
        .name(THREAD.into())
        .spawn(move || forward(&focus, &changes))?;
    let id = window.window_handle().window_id();
    cx.spawn(async move |cx| report(id, &heard, cx).await)
        .detach();
    Ok(())
}

fn forward(
    focus: &InputFocus,
    changes: &async_channel::Sender<FocusChange>,
) -> Result<(), async_channel::SendError<FocusChange>> {
    std::iter::from_fn(|| focus.next_change().ok())
        .try_for_each(|change| changes.send_blocking(change))
}

async fn report(
    window: WindowId,
    heard: &async_channel::Receiver<FocusChange>,
    cx: &mut AsyncApp,
) {
    while let Ok(change) = heard.recv().await {
        cx.update(|cx| apply(window, change, cx));
    }
}

fn apply(
    window: WindowId,
    change: FocusChange,
    cx: &mut App,
) {
    match change {
        FocusChange::Gained(_) => ui::WindowActivity::gained(window, cx),
        FocusChange::Lost(_) => ui::WindowActivity::lost(window, cx),
    }
}

#[cfg(test)]
mod tests {
    use gpui::{Empty, TestAppContext};

    use super::*;

    #[gpui::test]
    fn the_x_server_s_focus_changes_decide_whether_the_window_is_active(cx: &mut TestAppContext) {
        let window = cx.add_window(|_, _| Empty);
        let id = window.window_id();
        let mut seen = Vec::new();
        for change in [FocusChange::Lost(1), FocusChange::Gained(1)] {
            cx.update(|cx| apply(id, change, cx));
            let active = window.update(cx, |_, window, cx| ui::WindowActivity::of(window, cx));
            seen.push(active.unwrap_or_default());
        }
        assert_eq!(seen, [false, true]);
    }
}
