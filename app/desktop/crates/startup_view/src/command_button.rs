use std::rc::Rc;

use gpui::{App, IntoElement, RenderOnce, Window};
use ui::{Button, ButtonSize};

use crate::startup_actions::StartupActions;
use crate::startup_command::StartupCommand;

#[derive(IntoElement)]
pub(crate) struct CommandButton {
    command: StartupCommand,
    primary: StartupCommand,
    actions: Rc<dyn StartupActions>,
}

impl CommandButton {
    pub(crate) fn new(
        command: StartupCommand,
        primary: StartupCommand,
        actions: &Rc<dyn StartupActions>,
    ) -> Self {
        Self {
            command,
            primary,
            actions: Rc::clone(actions),
        }
    }
}

impl RenderOnce for CommandButton {
    fn render(self, _window: &mut Window, _cx: &mut App) -> impl IntoElement {
        let (command, actions) = (self.command, self.actions);
        Button::new(command.id(), command.label())
            .icon(command.icon())
            .size(ButtonSize::Md)
            .variant(command.variant(self.primary))
            .on_click(move |_, _, cx| actions.perform(command, cx))
    }
}

#[cfg(test)]
#[path = "command_button_tests.rs"]
mod tests;
