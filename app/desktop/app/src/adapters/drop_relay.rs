//! Drags from other applications onto the shell's web views: the window's drags go through a
//! proxy that hands them to the web view under the pointer (`platform::XdndProxy`).

use gpui::{App, Global, Window};
use platform::XdndProxy;

struct DropRelay {
    _proxy: XdndProxy,
}

impl Global for DropRelay {}

pub(crate) fn install(
    window: &Window,
    cx: &mut App,
) {
    if let Err(error) = start(window, cx) {
        tracing::warn!(%error, "drags cannot reach the web views");
    }
}

fn start(
    window: &Window,
    cx: &mut App,
) -> anyhow::Result<()> {
    let webview::ParentWindow::X11(xid) = webview::ParentWindow::from_window(window)? else {
        return Ok(());
    };
    cx.set_global(DropRelay {
        _proxy: XdndProxy::install(xid)?,
    });
    Ok(())
}

#[cfg(test)]
mod tests {
    use gpui::{Empty, TestAppContext};

    use super::*;

    #[gpui::test]
    fn a_window_without_an_x11_handle_gets_no_relay(cx: &mut TestAppContext) {
        let window = cx.add_window(|_, _| Empty);
        let relayed = window
            .update(cx, |_, window, cx| {
                install(window, cx);
                cx.has_global::<DropRelay>()
            })
            .unwrap_or_default();
        assert!(!relayed);
    }
}
