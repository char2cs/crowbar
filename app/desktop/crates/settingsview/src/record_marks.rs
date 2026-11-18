//! The small marks of a record row: its grip, its artwork and its status dot.

use std::sync::Arc;

use gpui::prelude::FluentBuilder as _;
use gpui::{ImageFormat, IntoElement, ParentElement, Styled, div, img, px};
use ui::{Icon, IconName, Palette};

use settings::RecordFlag;

pub(crate) const ICON: f32 = 18.0;
pub(crate) const FLAG_COLUMN: f32 = 64.0;
const GRIP: f32 = 16.0;
const DOT: f32 = 8.0;

pub(crate) fn grip(palette: &Palette) -> Icon {
    Icon::new(IconName::DotsSixVertical)
        .size(px(GRIP))
        .color(palette.muted_foreground())
}

pub(crate) fn artwork_image(markup: &str) -> impl IntoElement {
    let image = gpui::Image::from_bytes(ImageFormat::Svg, markup.as_bytes().to_vec());
    img(Arc::new(image)).size(px(ICON)).flex_none()
}

pub(crate) fn dot_box(
    flag: &RecordFlag,
    palette: &Palette,
) -> gpui::Div {
    let dot = div()
        .size(px(DOT))
        .rounded_full()
        .border_1()
        .border_color(palette.muted_foreground());
    let success = palette.success();
    let dot = dot.when(flag.on, |dot| dot.bg(success).border_color(success));
    div().flex().justify_center().w(px(FLAG_COLUMN)).child(dot)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;
    use ui::{Surface, Theme};

    use super::*;
    use crate::test_support::measure;

    #[gpui::test]
    fn a_status_dot_fills_the_flag_column_and_a_grip_has_its_size(cx: &mut TestAppContext) {
        let palette = *Theme::default().palette(Surface::Window);
        let flag = RecordFlag::new("connected", "Installed").on().as_dot();
        let column = measure(cx, move |_, _| dot_box(&flag, &palette));
        assert_eq!(f32::from(column.size.width), FLAG_COLUMN);
        let handle = measure(cx, move |_, _| grip(&palette));
        assert_eq!(f32::from(handle.size.width), GRIP);
    }
}
