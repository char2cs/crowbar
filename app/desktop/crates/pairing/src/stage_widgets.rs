//! The widgets the stages of the dialog are built from.

use gpui::{
    AnyElement, App, AppContext as _, ClickEvent, Context, Div, Entity, IntoElement, ParentElement,
    SharedString, Styled, Window, div,
};
use store::{PairingPending, PairingState};
use ui::{Button, ButtonSize, ButtonVariant, Palette, Surface, TextInput, TextInputStyle};

use crate::look::{FINGERPRINT_PAD, FINGERPRINT_TINT, GAP, LABEL_GAP, TEXT};
use crate::stage_copy;

pub(crate) fn field<Owner: 'static>(
    placeholder: &'static str,
    cx: &mut Context<'_, Owner>,
) -> Entity<TextInput> {
    cx.new(|cx| {
        TextInput::new(placeholder, cx)
            .on(Surface::Window)
            .style(TextInputStyle::Compact)
    })
}

pub(crate) fn muted(palette: &Palette) -> Div {
    div().text_size(TEXT).text_color(palette.muted_foreground())
}

pub(crate) fn labelled(
    label: &'static str,
    palette: &Palette,
    input: &Entity<TextInput>,
) -> Div {
    div()
        .flex()
        .flex_col()
        .gap(LABEL_GAP)
        .child(muted(palette).child(label))
        .child(input.clone())
}

pub(crate) fn fingerprint_box(
    palette: &Palette,
    fingerprint: &str,
    mono: &'static str,
) -> Div {
    div()
        .p(FINGERPRINT_PAD)
        .rounded_md()
        .bg(palette.accent().opacity(FINGERPRINT_TINT))
        .text_size(TEXT)
        .font_family(mono)
        .child(SharedString::from(fingerprint.to_owned()))
}

pub(crate) fn text_of<Owner: 'static>(
    input: &Entity<TextInput>,
    cx: &Context<'_, Owner>,
) -> String {
    input.read(cx).text().to_owned()
}

pub(crate) fn messages(
    state: &PairingState,
    palette: &Palette,
) -> Vec<AnyElement> {
    let notice = state
        .notice
        .as_ref()
        .map(|text| muted(palette).child(text.clone()).into_any_element());
    let error = state.error.as_ref().map(|text| {
        div()
            .text_size(TEXT)
            .text_color(palette.destructive())
            .child(text.clone())
            .into_any_element()
    });
    notice.into_iter().chain(error).collect()
}

pub(crate) fn footer(
    state: &PairingState,
    (on_primary, on_secondary): (
        impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
        impl Fn(&ClickEvent, &mut Window, &mut App) + 'static,
    ),
) -> Div {
    let busy = state.pending != PairingPending::Idle;
    let mut primary = Button::new("pair-primary", stage_copy::primary(state.stage))
        .size(ButtonSize::Sm)
        .on_click(on_primary);
    let mut secondary = Button::new("pair-secondary", stage_copy::secondary(state.stage))
        .variant(ButtonVariant::Ghost)
        .size(ButtonSize::Sm)
        .on_click(on_secondary);
    if busy {
        primary = primary.disabled();
        secondary = secondary.disabled();
    }
    div().flex().gap(GAP).child(secondary).child(primary)
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use super::*;

    struct Owner;

    #[gpui::test]
    fn a_new_field_is_empty_and_reads_back_what_is_typed(cx: &mut TestAppContext) {
        cx.update(|cx| ui::init(cx).unwrap());
        let owner = cx.new(|_| Owner);
        let input = owner.update(cx, |_, cx| field("Name", cx));
        owner.update(cx, |_, cx| {
            assert_eq!(text_of(&input, cx), "");
            input.update(cx, |input, cx| input.set_text("Lab", cx));
            assert_eq!(text_of(&input, cx), "Lab");
        });
    }
}
