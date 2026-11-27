use gpui::{Div, FontWeight, ParentElement, Styled, div};
use ui::{Palette, TrackedText, tracking};

use crate::sidebar::scale::TEXT_MICRO;

const LABEL: &str = "ICON";

pub(super) fn section_label(palette: &Palette) -> Div {
    spaced_label(palette, tracking::WIDER)
}

fn spaced_label(
    palette: &Palette,
    em: f32,
) -> Div {
    div()
        .text_size(TEXT_MICRO)
        .font_weight(FontWeight::SEMIBOLD)
        .text_color(palette.muted_foreground())
        .child(TrackedText::new(LABEL, em))
}

#[cfg(test)]
mod tests {
    use gpui::{TestAppContext, px};
    use ui::{Surface, Theme};

    use super::*;

    const WIDER: f32 = 0.05;

    fn label_width(
        cx: &mut TestAppContext,
        em: f32,
    ) -> f32 {
        let window = crate::test_support::mount(cx, move |_, cx| {
            let label = spaced_label(Theme::of(cx).palette(Surface::Window), em);
            div().flex().child(
                gpui::InteractiveElement::debug_selector(div().flex_none(), || "label".into())
                    .child(label),
            )
        });
        f32::from(window.debug_bounds("label").unwrap_or_default().size.width)
    }

    #[gpui::test]
    fn the_section_label_is_set_wider_as_develop_does(cx: &mut TestAppContext) {
        let wider = label_width(cx, WIDER);
        let loose = label_width(cx, 0.0);
        let glyphs = f32::from(u16::try_from(LABEL.chars().count()).expect("a short label"));
        let expected = glyphs * WIDER * f32::from(TEXT_MICRO);
        assert!(
            (wider - loose - expected).abs() < 1.0,
            "{wider} - {loose} vs {expected}"
        );
    }

    const DEVELOP_LABEL: f32 = 10.0;

    #[gpui::test]
    fn the_section_label_is_set_at_develops_ten_pixels(cx: &mut TestAppContext) {
        let label = label_width(cx, 0.0);
        let window = crate::test_support::mount(cx, move |_, _| {
            let develop = div()
                .text_size(px(DEVELOP_LABEL))
                .font_weight(FontWeight::SEMIBOLD)
                .child(LABEL);
            div().flex().child(
                gpui::InteractiveElement::debug_selector(div().flex_none(), || "develop".into())
                    .child(develop),
            )
        });
        let develop = f32::from(
            window
                .debug_bounds("develop")
                .unwrap_or_default()
                .size
                .width,
        );
        assert!(
            (label - develop).abs() < 0.5,
            "{label} vs develop {develop}"
        );
    }
}
