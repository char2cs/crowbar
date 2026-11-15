//! The 36px capsule every tree row is drawn in.

use gpui::{Div, ElementId, InteractiveElement, Stateful, Styled, div, px, transparent_black};
use ui::{Palette, radius};

use crate::metrics::{INDENT_STEP, ROW_HEIGHT, ROW_MARGIN_X, ROW_MARGIN_Y};

const GAP: f32 = 6.0;
const PADDING: f32 = 6.0;

pub(crate) fn row_shell(
    id: ElementId,
    depth: u8,
    palette: &Palette,
) -> Stateful<Div> {
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
mod tests {
    use gpui::{ParentElement, TestAppContext};
    use ui::{Surface, Theme};

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn a_row_is_the_spec_height_plus_its_vertical_margin(cx: &mut TestAppContext) {
        let bounds = measure(cx, |_, cx| {
            let palette = Theme::of(cx).palette(Surface::Chrome);
            row_shell("row".into(), 0, palette).child("label")
        });
        assert_eq!(f32::from(bounds.size.height), ROW_HEIGHT + ROW_MARGIN_Y);
    }
}
