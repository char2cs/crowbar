//! The description under an alert dialog's title: 8px below it, the header's padding beneath.

use gpui::{AnyElement, Div, ParentElement, Styled, div};

use crate::floating::dialog::dialog_style::DialogStyle;
use crate::foundation::Palette;
use crate::foundation::tokens::spacing;
use crate::foundation::tokens::text_size;

pub(crate) fn description_block(
    description: Option<AnyElement>,
    (palette, style): (&Palette, DialogStyle),
) -> Option<Div> {
    let description = description?;
    let (x, y) = style.header_padding();
    let block = div()
        .flex_none()
        .px(x)
        .pt(spacing::STEP_2)
        .pb(y)
        .text_size(text_size::SM)
        .line_height(text_size::LINE_HEIGHT)
        .text_color(palette.muted_foreground())
        .child(description);
    Some(block)
}

#[cfg(test)]
mod tests {
    use gpui::{IntoElement, TestAppContext};

    use super::*;
    use crate::foundation::{Surface, Theme};
    use crate::test_support::probe::open;

    #[gpui::test]
    fn sits_8px_under_the_title_with_the_header_padding_below(cx: &mut TestAppContext) {
        let palette = Theme::default().palette(Surface::Window);
        let cx = open(cx, move |_, _| {
            div().children(description_block(
                Some("Gone.".into_any_element()),
                (palette, DialogStyle::Alert),
            ))
        });
        let bounds = cx.debug_bounds("probe").unwrap_or_default();
        assert_eq!(f32::from(bounds.size.height), 8.0 + 20.0 + 24.0);
    }
}
