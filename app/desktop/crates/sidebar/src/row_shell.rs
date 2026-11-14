//! The 36px capsule every tree row is drawn in.

use gpui::{Div, ElementId, InteractiveElement, Stateful, Styled, div, px, transparent_black};
use ui::{Palette, radius};

use crate::metrics::{INDENT_STEP, ROW_HEIGHT, ROW_MARGIN_X, ROW_MARGIN_Y};

const GAP: f32 = 6.0;
const PADDING: f32 = 6.0;

pub(crate) fn row_shell(id: ElementId, depth: u8, palette: &Palette) -> Stateful<Div> {
    div()
        .id(id)
        .relative()
        .flex()
        .items_center()
        .gap(px(GAP))
        .h(px(ROW_HEIGHT))
        .ml(px(ROW_MARGIN_X + f32::from(depth) * INDENT_STEP))
        .mr(px(ROW_MARGIN_X))
        .mt(px(ROW_MARGIN_Y))
        .px(px(PADDING))
        .rounded(radius::LG)
        .border_1()
        .border_color(transparent_black())
        .text_color(palette.foreground())
}

#[cfg(test)]
#[path = "row_shell_tests.rs"]
mod tests;
