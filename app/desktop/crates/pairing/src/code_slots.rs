use gpui::prelude::FluentBuilder as _;
use gpui::{
    App, BoxShadow, Div, Entity, Focusable as _, InteractiveElement as _, IntoElement,
    ParentElement, Pixels, RenderOnce, SharedString, Styled, Window, div, point, px,
};
use ui::{Palette, Surface, TextInput, Theme, radius};

pub(crate) const CODE_LENGTH: usize = 6;
const SLOT: Pixels = px(32.0);
const SLOT_GAP: Pixels = px(8.0);
const SLOT_TEXT: Pixels = px(14.0);
const RULE: (Pixels, Pixels) = (px(12.0), px(2.0));
const RING_WIDTH: Pixels = px(3.0);
const RING_ALPHA: f32 = 0.24;
const SPLIT_AT: usize = 3;

#[derive(IntoElement)]
pub(crate) struct CodeSlots {
    pub(crate) input: Entity<TextInput>,
}

impl RenderOnce for CodeSlots {
    fn render(
        self,
        window: &mut Window,
        cx: &mut App,
    ) -> impl IntoElement {
        let palette = *Theme::of(cx).palette(Surface::Window);
        let field = self.input.read(cx);
        let chars: Vec<char> = field.text().chars().collect();
        let focused = field.focus_handle(cx).is_focused(window);
        let next = focused.then(|| chars.len().min(CODE_LENGTH - 1));
        let slot = |at: usize| {
            slot(chars.get(at).copied(), &palette)
                .when(next == Some(at), |slot| ringed(slot, &palette))
        };
        let (rule_width, rule_height) = RULE;
        let rule = div()
            .w(rule_width)
            .h(rule_height)
            .rounded_full()
            .bg(palette.input());
        div()
            .relative()
            .flex()
            .items_center()
            .gap(SLOT_GAP)
            .debug_selector(|| "pairing-code-slots".into())
            .children((0..SPLIT_AT).map(slot))
            .child(rule)
            .children((SPLIT_AT..CODE_LENGTH).map(slot))
            .child(div().absolute().inset_0().opacity(0.0).child(self.input))
    }
}

fn ringed(
    slot: Div,
    palette: &Palette,
) -> Div {
    let ring = BoxShadow {
        color: palette.ring().opacity(RING_ALPHA),
        offset: point(px(0.0), px(0.0)),
        blur_radius: px(0.0),
        spread_radius: RING_WIDTH,
        inset: false,
    };
    slot.border_color(palette.ring()).shadow(vec![ring])
}

fn slot(
    shown: Option<char>,
    palette: &Palette,
) -> Div {
    div()
        .debug_selector(|| "pairing-code-slot".into())
        .size(SLOT)
        .flex()
        .items_center()
        .justify_center()
        .rounded(radius::LG)
        .border_1()
        .border_color(palette.input())
        .bg(palette.background())
        .text_size(SLOT_TEXT)
        .text_color(palette.foreground())
        .children(shown.map(|shown| SharedString::from(shown.to_string())))
}

pub(crate) fn overrun(typed: &str) -> Option<String> {
    let fits = typed.chars().count() <= CODE_LENGTH;
    (!fits).then(|| typed.chars().take(CODE_LENGTH).collect())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_code_longer_than_six_keeps_its_first_six_characters() {
        let cases = [
            ("12345", None),
            ("123456", None),
            ("1234567", Some("123456")),
        ];
        for (typed, kept) in cases {
            assert_eq!(overrun(typed).as_deref(), kept, "{typed}");
        }
    }
}
