use gpui::TestAppContext;

use crate::sidebar_state::SidebarState;
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
            left + 8.0 + 34.0,
            "after the 28px toggle and 6px gap"
        );
        assert_eq!(
            bar.strip_width(),
            width - 16.0 - 34.0,
            "toggle and gap come off the width"
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
