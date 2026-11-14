use gpui::App;

use crate::startup_command::StartupCommand;

/// What the shell does for the commands of the startup view; the view only asks.
pub trait StartupActions {
    /// Carries out `command`.
    fn perform(&self, command: StartupCommand, cx: &mut App);
}

#[cfg(test)]
#[path = "startup_actions_tests.rs"]
mod tests;
