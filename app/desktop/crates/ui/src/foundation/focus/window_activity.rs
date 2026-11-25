//! Whether the keyboard is in a window, one of the window's own child windows included.

use std::collections::HashMap;

use gpui::{App, Global, Window, WindowId};

const ACTIVE: bool = true;

/// Per window, whether the keyboard is in it or in one of its child windows (a web view), as the
/// platform reports it; read it instead of `Window::is_window_active`.
///
/// Stock GPUI counts the keyboard moving into a child window as the window going inactive.
/// Until a platform reports a window, it falls back to GPUI's own answer.
#[derive(Debug, Default)]
pub struct WindowActivity {
    reported: HashMap<WindowId, bool>,
}

impl Global for WindowActivity {}

impl WindowActivity {
    /// Whether `window` holds the keyboard, itself or through one of its child windows.
    #[must_use]
    pub fn of(
        window: &Window,
        cx: &App,
    ) -> bool {
        let id = window.window_handle().window_id();
        cx.try_global::<Self>()
            .and_then(|activity| activity.reported.get(&id).copied())
            .unwrap_or_else(|| window.is_window_active())
    }

    /// Records the platform's report that the keyboard came into `window`, one of its child
    /// windows included; notifies this global's observers and redraws every window.
    pub fn gained(
        window: WindowId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().reported.insert(window, ACTIVE);
        cx.refresh_windows();
    }

    /// Records the platform's report that the keyboard left `window` and its child windows;
    /// notifies this global's observers and redraws every window.
    pub fn lost(
        window: WindowId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().reported.insert(window, !ACTIVE);
        cx.refresh_windows();
    }
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, div};

    use super::*;
    use crate::test_support::probe::open;

    #[gpui::test]
    fn a_report_outranks_gpui_s_own_answer(cx: &mut TestAppContext) {
        let cx = open(cx, |_, _| div());
        cx.deactivate_window();
        let unreported = cx.update(|window, cx| WindowActivity::of(window, cx));
        cx.update(|window, cx| {
            WindowActivity::gained(window.window_handle().window_id(), cx);
        });
        let reported = cx.update(|window, cx| WindowActivity::of(window, cx));
        assert_eq!((unreported, reported), (false, true));
    }
}
