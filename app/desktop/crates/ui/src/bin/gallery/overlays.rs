//! Menu, dropdown and dialog.

use gpui::{IntoElement, ParentElement, Styled, div, px};
use ui::{Button, ButtonVariant, Dialog, DropdownMenu, IconName, MenuAction, MenuItem};

use crate::section::row;

fn items() -> Vec<MenuItem> {
    vec![
        MenuItem::Action(
            MenuAction::new("Rename", |_, _| {})
                .icon(IconName::PencilSimpleLine)
                .shortcut("F2"),
        ),
        MenuItem::Action(MenuAction::new("Copy path", |_, _| {}).icon(IconName::Copy)),
        MenuItem::Separator,
        MenuItem::Label("Danger".into()),
        MenuItem::Action(
            MenuAction::new("Delete", |_, _| {})
                .icon(IconName::Trash)
                .destructive(),
        ),
        MenuItem::Action(MenuAction::new("Archive", |_, _| {}).disabled()),
    ]
}

pub(crate) fn menus() -> impl IntoElement {
    let trigger = Button::new("dropdown-trigger", "Open menu").variant(ButtonVariant::Outline);
    row().child(DropdownMenu::new("dropdown", trigger, items()))
}

pub(crate) fn dialog() -> impl IntoElement {
    let actions = div()
        .flex()
        .gap(px(8.0))
        .child(Button::new("cancel", "Cancel").variant(ButtonVariant::Outline))
        .child(Button::new("confirm", "Delete").variant(ButtonVariant::Destructive));
    Dialog::new("dialog")
        .title("Delete space")
        .icon(IconName::Warning)
        .body("This removes the space and its chats.")
        .footer(actions)
}
