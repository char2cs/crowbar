use gpui::{ParentElement, Styled, TestAppContext, div, px};

use super::*;
use crate::surface::Surface;
use crate::test_support::open;
use crate::theme::Theme;

#[gpui::test]
fn pill_is_one_slot_wide_and_moves_with_the_position(cx: &mut TestAppContext) {
    let palette = Theme::default().palette(Surface::Window);
    let cx = open(cx, move |_, _| {
        let pill = tab_indicator(TabsStyle::Pill, palette, 4, 2.0);
        div().relative().w(px(400.0)).h(px(32.0)).child(pill)
    });
    let slot = cx.debug_bounds("tab-indicator-slot").unwrap_or_default();
    let frame = cx.debug_bounds("probe").unwrap_or_default();
    assert!((f32::from(slot.size.width) - 100.5).abs() < 0.5, "{slot:?}");
    assert!(
        (f32::from(slot.origin.x - frame.origin.x) - 201.0).abs() < 0.5,
        "{slot:?} in {frame:?}"
    );
}
