//! The confirmation dialog.

use gpui::{Entity, IntoElement, ParentElement, Styled, div, px};
use ui::{Button, ButtonSize, ButtonVariant, Dialog};

use crate::dispatch::send;
use crate::intent::Intent;
use crate::prompt::Prompt;
use crate::settings_modal::SettingsModal;

pub(crate) fn prompt_view(owner: &Entity<SettingsModal>, prompt: &Prompt) -> impl IntoElement {
    let buttons = div()
        .flex()
        .justify_end()
        .gap(px(8.0))
        .child(
            Button::new("prompt-cancel", "Cancel")
                .variant(ButtonVariant::Outline)
                .size(ButtonSize::Sm)
                .on_click(send(owner, Intent::Dismiss)),
        )
        .child(
            Button::new("prompt-accept", prompt.confirm.action.clone())
                .variant(ButtonVariant::Destructive)
                .size(ButtonSize::Sm)
                .on_click(send(owner, Intent::Accept)),
        );
    let dismiss = {
        let owner = owner.clone();
        move |window: &mut gpui::Window, cx: &mut gpui::App| {
            crate::dispatch::deliver(&owner, Intent::Dismiss, (window, cx));
        }
    };
    Dialog::new("settings-confirm")
        .title(prompt.title().to_owned())
        .body(prompt.message().to_owned())
        .footer(buttons)
        .on_close(dismiss)
}

#[cfg(test)]
#[path = "prompt_view_tests.rs"]
mod tests;
