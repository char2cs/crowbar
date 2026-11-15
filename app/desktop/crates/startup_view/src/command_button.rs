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
    fn render(
        self,
        _window: &mut Window,
        _cx: &mut App,
    ) -> impl IntoElement {
        let (command, actions) = (self.command, self.actions);
        Button::new(command.id(), command.label())
            .icon(command.icon())
            .size(ButtonSize::Md)
            .variant(command.variant(self.primary))
            .on_click(move |_, _, cx| actions.perform(command, cx))
    }
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::{Modifiers, TestAppContext, point, px};

    use super::*;
    use crate::test_support::{RecordingActions, open};

    #[gpui::test]
    fn a_click_performs_the_command(cx: &mut TestAppContext) {
        let actions = Rc::new(RecordingActions::default());
        let shared = Rc::clone(&actions) as Rc<dyn StartupActions>;
        let cx = open(cx, move |_, _| {
            CommandButton::new(StartupCommand::OpenLog, StartupCommand::Quit, &shared)
        });
        cx.simulate_click(point(px(20.0), px(16.0)), Modifiers::none());
        assert_eq!(actions.performed(), [StartupCommand::OpenLog]);
    }
}
