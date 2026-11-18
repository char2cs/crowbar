use gpui::App;

use crate::startup_command::StartupCommand;

/// What the shell does for the commands of the startup view; the view only asks.
pub trait StartupActions {
    /// Carries out `command`.
    fn perform(
        &self,
        command: StartupCommand,
        cx: &mut App,
    );
}

#[cfg(test)]
mod tests {
    use std::rc::Rc;

    use gpui::TestAppContext;

    use super::*;
    use crate::test_support::RecordingActions;

    #[gpui::test]
    fn actions_are_usable_as_a_shared_trait_object(cx: &mut TestAppContext) {
        let recording = Rc::new(RecordingActions::default());
        let actions = Rc::clone(&recording) as Rc<dyn StartupActions>;
        cx.update(|cx| actions.perform(StartupCommand::OpenLog, cx));
        assert_eq!(recording.performed(), [StartupCommand::OpenLog]);
    }
}
