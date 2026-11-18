//! Every button variant, size and state.

use gpui::{IntoElement, ParentElement, Styled, div};
use ui::{Button, ButtonSize, ButtonVariant, IconName, spacing};

use crate::page::section::row;

const VARIANTS: [(ButtonVariant, &str); 5] = [
    (ButtonVariant::Primary, "Primary"),
    (ButtonVariant::Destructive, "Delete"),
    (ButtonVariant::Outline, "Outline"),
    (ButtonVariant::Secondary, "Secondary"),
    (ButtonVariant::Ghost, "Ghost"),
];
const SIZES: [ButtonSize; 5] = [
    ButtonSize::Xs,
    ButtonSize::Sm,
    ButtonSize::Md,
    ButtonSize::Lg,
    ButtonSize::Xl,
];

fn variants() -> impl IntoElement {
    let buttons = VARIANTS
        .iter()
        .enumerate()
        .map(|(index, (variant, label))| Button::new(("variant", index), *label).variant(*variant));
    row().children(buttons)
}

fn disabled() -> impl IntoElement {
    let buttons = VARIANTS
        .iter()
        .enumerate()
        .map(|(index, (variant, label))| {
            Button::new(("off", index), *label)
                .variant(*variant)
                .disabled()
        });
    row().children(buttons)
}

fn loading() -> impl IntoElement {
    let buttons = VARIANTS
        .iter()
        .enumerate()
        .map(|(index, (variant, label))| {
            Button::new(("busy", index), *label)
                .variant(*variant)
                .loading()
        });
    row().children(buttons)
}

fn sizes() -> impl IntoElement {
    let buttons = SIZES.iter().enumerate().map(|(index, size)| {
        Button::new(("size", index), "Button")
            .size(*size)
            .variant(ButtonVariant::Outline)
    });
    row().children(buttons)
}

fn icon_buttons() -> impl IntoElement {
    let buttons = SIZES
        .iter()
        .enumerate()
        .map(|(index, size)| Button::icon_only(("icon", index), IconName::Gear).size(*size));
    row()
        .children(buttons)
        .child(Button::new("add", "Add space").icon(IconName::Plus))
}

pub(crate) fn buttons() -> impl IntoElement {
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_2)
        .child(variants())
        .child(disabled())
        .child(loading())
        .child(sizes())
        .child(icon_buttons())
}
