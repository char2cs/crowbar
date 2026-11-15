use gpui::{Pixels, px};

use crate::sidebar_state::SidebarState;
use crate::span::Span;
use crate::strip_metrics::{ROW_GAP, ROW_PADDING, TOGGLE_SIZE, TRAFFIC_LIGHT_INSET, slots};
use crate::tab_bar::TabBar;

impl TabBar {
    pub(crate) fn strip_left(&self) -> Pixels {
        let toggle = match self.sidebar {
            SidebarState::ClosedLeft => TOGGLE_SIZE + ROW_GAP + TRAFFIC_LIGHT_INSET,
            SidebarState::Open | SidebarState::ClosedRight => 0.0,
        };
        self.row.origin.x + px(ROW_PADDING + toggle)
    }

    pub(crate) fn strip_width(&self) -> f32 {
        let toggle = match self.sidebar {
            SidebarState::Open => 0.0,
            SidebarState::ClosedRight => TOGGLE_SIZE + ROW_GAP,
            SidebarState::ClosedLeft => TOGGLE_SIZE + ROW_GAP + TRAFFIC_LIGHT_INSET,
        };
        (f32::from(self.row.size.width) - 2.0 * ROW_PADDING - toggle).max(0.0)
    }

    pub(crate) fn slot_spans(&self) -> Vec<Span> {
        slots(self.tabs.len(), self.strip_width())
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::sidebar_state::SidebarState;
    use crate::strip_metrics::TRAFFIC_LIGHT_INSET;
    use crate::test_support::{open, view};

    #[gpui::test]
    fn the_strip_starts_after_the_padding_and_the_collapsed_toggle(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        bar.read_with(cx, |bar, _| {
            let (left, width) = (f32::from(bar.row.origin.x), f32::from(bar.row.size.width));
            assert_eq!(f32::from(bar.strip_left()), left + 8.0, "8px padding");
            assert_eq!(bar.strip_width(), width - 16.0, "padding on both sides");
        });
        bar.update(cx, |bar, cx| bar.set_sidebar(SidebarState::ClosedLeft, cx));
        cx.run_until_parked();
        bar.read_with(cx, |bar, _| {
            let (left, width) = (f32::from(bar.row.origin.x), f32::from(bar.row.size.width));
            assert_eq!(
                f32::from(bar.strip_left()),
                left + 8.0 + 34.0 + TRAFFIC_LIGHT_INSET,
                "after the traffic lights, the 28px toggle and 6px gap"
            );
            assert_eq!(
                bar.strip_width(),
                width - 16.0 - 34.0 - TRAFFIC_LIGHT_INSET,
                "inset, toggle and gap come off the width"
            );
        });
    }

    #[gpui::test]
    fn a_lone_tab_is_240_wide_and_many_tabs_share_the_strip(cx: &mut TestAppContext) {
        let harness = open(cx, vec![view(1, &["a"])], 1);
        let (bar, cx) = (harness.bar, harness.cx);
        bar.read_with(cx, |bar, _| {
            assert_eq!(bar.slot_spans()[0].width, 240.0, "lone tab");
        });
        bar.update(cx, |bar, cx| {
            let tabs = (1..=40).map(|id| view(id, &["x"])).collect();
            bar.set_tabs(tabs, None, cx);
        });
        cx.run_until_parked();
        bar.read_with(cx, |bar, _| {
            assert_eq!(bar.slot_spans()[0].width, 112.0, "a crowded strip scrolls");
        });
    }
}
