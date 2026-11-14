//! The pill that slides behind the selected tab.

use gpui::{Div, InteractiveElement as _, ParentElement, Styled, div, px, relative};

use crate::palette::Palette;
use crate::tabs_style::TabsStyle;

pub(crate) const GAP: f32 = 2.0;

pub(crate) fn tab_indicator(style: TabsStyle, palette: &Palette, count: u16, position: f32) -> Div {
    let slots = f32::from(count.max(1));
    let pill = style.pill(palette).h_full().mr(px(GAP));
    let slot = div()
        .absolute()
        .top_0()
        .h_full()
        .w(relative(1.0 / slots))
        .left(relative(position / slots))
        .debug_selector(|| "tab-indicator-slot".into())
        .child(pill);
    div()
        .absolute()
        .top_0()
        .bottom_0()
        .left_0()
        .right(px(-GAP))
        .child(slot)
}

#[cfg(test)]
#[path = "tab_indicator_tests.rs"]
mod tests;
