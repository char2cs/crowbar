//! Menu, dropdown and dialog.

use gpui::{IntoElement, ParentElement, Pixels, Styled, div, px};
use ui::{
    Button, ButtonVariant, ContextMenu, Dialog, DropdownMenu, IconName, MenuAction, MenuItem,
    Popover, spacing,
};

use crate::section::row;

const AREA_WIDTH: Pixels = px(220.0);
const AREA_HEIGHT: Pixels = px(64.0);
const POPOVER_WIDTH: Pixels = px(200.0);

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
        MenuItem::Action(
            MenuAction::submenu(
                "Move to",
                vec![
                    MenuItem::Action(MenuAction::new("Main", |_, _| {})),
                    MenuItem::Action(MenuAction::new("Review", |_, _| {})),
                ],
            )
            .icon(IconName::Folder),
        ),
    ]
}

pub(crate) fn menus() -> impl IntoElement {
    let trigger = Button::new("dropdown-trigger", "Open menu").variant(ButtonVariant::Outline);
    let area = div()
        .flex()
        .items_center()
        .justify_center()
        .w(AREA_WIDTH)
        .h(AREA_HEIGHT)
        .border_1()
        .rounded(spacing::STEP_2)
        .child("Right-click here");
    let popover = Popover::new(
        "popover",
        Button::new("popover-trigger", "Popover").variant(ButtonVariant::Outline),
        |_, _| {
            div()
                .w(POPOVER_WIDTH)
                .child("Any content in a popup frame.")
        },
    );
    row()
        .child(DropdownMenu::new("dropdown", trigger, items()))
        .child(ContextMenu::new("context", area, items()))
        .child(popover)
}

pub(crate) fn dialog() -> impl IntoElement {
    let actions = div()
        .flex()
        .gap(spacing::STEP_2)
        .child(Button::new("cancel", "Cancel").variant(ButtonVariant::Outline))
        .child(Button::new("confirm", "Delete").variant(ButtonVariant::Destructive));
    Dialog::new("dialog")
        .title("Delete space")
        .icon(IconName::Warning)
        .body("This removes the space and its chats.")
        .footer(actions)
}
