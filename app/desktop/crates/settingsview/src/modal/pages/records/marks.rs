use gpui::prelude::FluentBuilder as _;
use gpui::{ParentElement, Styled, div, px};
use ui::{Icon, IconName, Palette, ProviderGlyph};

use settings::RecordFlag;

pub(crate) const ICON: f32 = 18.0;
pub(crate) const FLAG_COLUMN: f32 = 64.0;
const GRIP: f32 = 16.0;
const ARTWORK: f32 = 14.0;
const DOT: f32 = 8.0;

pub(crate) fn grip(palette: &Palette) -> Icon {
    Icon::new(IconName::DotsSixVertical)
        .size(px(GRIP))
        .color(palette.muted_foreground())
}

pub(crate) fn artwork_image(
    markup: &str,
    palette: &Palette,
) -> ProviderGlyph {
    ProviderGlyph::new(markup.to_owned(), palette.foreground()).size(px(ARTWORK))
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
    let dot = dot.when(flag.is_on(), |dot| dot.bg(success).border_color(success));
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
