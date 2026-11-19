//! Badges, toasts, skeleton, spinner and empty state.

use gpui::{IntoElement, ParentElement, Pixels, Styled, div, px};
use ui::{
    Badge, BadgeSize, BadgeVariant, EmptyState, FlickerSpinner, IconName, Kbd, Skeleton, Spinner,
    Toast, ToastKind, spacing,
};

use crate::page::section::row;

const TOAST_WIDTH: Pixels = px(288.0);
const EMPTY_WIDTH: Pixels = px(240.0);
const SKELETON_WIDTH: Pixels = px(200.0);

const BADGES: [(BadgeVariant, &str); 8] = [
    (BadgeVariant::Primary, "3"),
    (BadgeVariant::Secondary, "12"),
    (BadgeVariant::Destructive, "9"),
    (BadgeVariant::Error, "err"),
    (BadgeVariant::Info, "info"),
    (BadgeVariant::Success, "ok"),
    (BadgeVariant::Warning, "warn"),
    (BadgeVariant::Outline, "out"),
];
const TOASTS: [ToastKind; 5] = [
    ToastKind::Info,
    ToastKind::Success,
    ToastKind::Warning,
    ToastKind::Error,
    ToastKind::Loading,
];

fn badges() -> impl IntoElement {
    row().children(
        BADGES
            .iter()
            .map(|(variant, text)| Badge::new(*text).variant(*variant)),
    )
}

fn extras() -> impl IntoElement {
    row()
        .child(Badge::new("sm").size(BadgeSize::Sm))
        .child(Badge::new("md").size(BadgeSize::Md))
        .child(Badge::new("lg").size(BadgeSize::Lg))
        .child(Kbd::new("Ctrl+B"))
        .child(Spinner::new("spinner"))
        .child(FlickerSpinner::new("flicker-spinner"))
}

fn toasts() -> impl IntoElement {
    let cards = TOASTS
        .iter()
        .enumerate()
        .map(|(index, kind)| Toast::new(("toast", index), *kind, "Branch created"));
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_2)
        .w(TOAST_WIDTH)
        .children(cards)
}

fn empty() -> impl IntoElement {
    let state = EmptyState::new("No chats yet")
        .icon(IconName::ChatsCircle)
        .description("Start one from the New Tab page");
    div().w(EMPTY_WIDTH).child(state)
}

pub(crate) fn feedback() -> impl IntoElement {
    let column = div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_2)
        .child(badges())
        .child(extras())
        .child(Skeleton::new("skeleton", SKELETON_WIDTH, spacing::STEP_3_5));
    div()
        .flex()
        .gap(spacing::STEP_6)
        .child(column)
        .child(toasts())
        .child(empty())
}
