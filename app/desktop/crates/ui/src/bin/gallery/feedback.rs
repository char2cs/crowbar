//! Badges, toasts, skeletons and other passive components.

use gpui::{Context, IntoElement, ParentElement, Styled, div, px};
use ui::{Badge, BadgeSize, BadgeVariant, EmptyState, IconName, Kbd, Skeleton, Spinner, Toast, ToastKind};

use crate::page::Page;

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

impl Page {
    pub(crate) fn feedback(&self, _cx: &Context<Self>) -> impl IntoElement {
        let badges = BADGES.iter().fold(div().flex().items_center().gap(px(8.0)), |row, (variant, text)| {
            row.child(Badge::new(*text).variant(*variant))
        });
        let sizes = div()
            .flex()
            .items_center()
            .gap(px(8.0))
            .child(Badge::new("sm").size(BadgeSize::Sm))
            .child(Badge::new("md").size(BadgeSize::Md))
            .child(Badge::new("lg").size(BadgeSize::Lg))
            .child(Kbd::new("Ctrl+B"))
            .child(Spinner::new("spinner"));
        let toasts = [ToastKind::Info, ToastKind::Success, ToastKind::Warning, ToastKind::Error, ToastKind::Loading]
            .into_iter()
            .enumerate()
            .fold(div().flex().flex_col().gap(px(8.0)).w(px(288.0)), |column, (index, kind)| {
                column.child(Toast::new(("toast", index), kind, "Branch created"))
            });
        div()
            .flex()
            .gap(px(24.0))
            .child(div().flex().flex_col().gap(px(8.0)).child(badges).child(sizes).child(Skeleton::new("skeleton", px(200.0), px(14.0))))
            .child(toasts)
            .child(div().w(px(240.0)).child(EmptyState::new("No chats yet").icon(IconName::ChatsCircle).description("Start one from the New Tab page")))
    }
}
