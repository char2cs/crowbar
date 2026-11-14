//! Menu, dropdown and dialog.

use gpui::prelude::FluentBuilder;
use gpui::{Context, IntoElement, ParentElement, Styled, Window, div, px};
use ui::{Button, ButtonVariant, Dialog, DropdownMenu, IconName, MenuAction, MenuItem};

use crate::page::Page;

fn items() -> Vec<MenuItem> {
    vec![
        MenuItem::Action(MenuAction::new("Rename", |_, _| {}).icon(IconName::PencilSimpleLine).shortcut("F2")),
        MenuItem::Action(MenuAction::new("Copy path", |_, _| {}).icon(IconName::Copy)),
        MenuItem::Separator,
        MenuItem::Label("Danger".into()),
        MenuItem::Action(MenuAction::new("Delete", |_, _| {}).icon(IconName::Trash).destructive(true)),
        MenuItem::Action(MenuAction::new("Archive", |_, _| {}).disabled(true)),
    ]
}

impl Page {
    pub(crate) fn overlays(&self, _window: &mut Window, _cx: &Context<Self>) -> impl IntoElement {
        let dropdown = DropdownMenu::new("dropdown", Button::new("dropdown-trigger", "Open menu").variant(ButtonVariant::Outline), items());
        let dialog = Dialog::new("dialog")
            .title("Delete space")
            .icon(IconName::Warning)
            .body(div().child("This removes the space and its chats."))
            .footer(div().flex().gap(px(8.0)).child(Button::new("cancel", "Cancel").variant(ButtonVariant::Outline)).child(Button::new("confirm", "Delete").variant(ButtonVariant::Destructive)));
        let row = div().flex().gap(px(16.0)).child(dropdown);
        row.when(self.only.as_deref() == Some("dialog"), |row| row.child(dialog))
    }
}
