use std::collections::HashMap;

use gpui::{App, Global, Window, WindowId};

const ACTIVE: bool = true;

#[derive(Debug, Default)]
pub struct WindowActivity {
    reported: HashMap<WindowId, bool>,
}

impl Global for WindowActivity {}

impl WindowActivity {
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

    #[must_use]
    pub fn is_reported(
        window: &Window,
        cx: &App,
    ) -> bool {
        let id = window.window_handle().window_id();
        cx.try_global::<Self>()
            .is_some_and(|activity| activity.reported.contains_key(&id))
    }

    pub fn gained(
        window: WindowId,
        cx: &mut App,
    ) {
        cx.default_global::<Self>().reported.insert(window, ACTIVE);
        cx.refresh_windows();
    }

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
        assert!(cx.update(|window, cx| WindowActivity::is_reported(window, cx)));
    }
}
