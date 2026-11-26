use gpui::Context;

use crate::daemon::daemon_link::DaemonLink;
use crate::daemon::startup::startup_action::StartupAction;
use crate::shell::inbox::logged::emit;

mod log_open_event;
use log_open_event::LogOpenEvent;

#[cfg(target_os = "macos")]
const OPENER: &str = "open";
#[cfg(not(target_os = "macos"))]
const OPENER: &str = "xdg-open";

pub(crate) fn perform<V: 'static>(
    command: startup::Command,
    link: &DaemonLink,
    cx: &mut Context<'_, V>,
) {
    match action_for(command) {
        StartupAction::ShowLog => open_log(link),
        StartupAction::QuitApp => cx.quit(),
        StartupAction::Nothing => {},
    }
}

fn action_for(command: startup::Command) -> StartupAction {
    match command {
        startup::Command::OpenLog => StartupAction::ShowLog,
        startup::Command::Quit => StartupAction::QuitApp,
        _ => StartupAction::Nothing,
    }
}

fn open_log(link: &DaemonLink) {
    let Some(log) = link.log() else {
        return emit(&LogOpenEvent::NoLog);
    };
    if let Err(error) = std::process::Command::new(OPENER).arg(&log).spawn() {
        emit(&LogOpenEvent::Unopened { error: &error });
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, Context, TestAppContext};

    use super::*;
    use crate::test_support::link_without_daemon;

    struct Empty;

    #[test]
    fn each_command_maps_to_its_action() {
        const CASES: [(startup::Command, StartupAction); 2] = [
            (startup::Command::OpenLog, StartupAction::ShowLog),
            (startup::Command::Quit, StartupAction::QuitApp),
        ];
        for (command, action) in CASES {
            assert_eq!(action_for(command), action);
        }
    }

    #[gpui::test]
    fn opening_a_log_that_does_not_exist_is_harmless(cx: &mut TestAppContext) {
        let link = link_without_daemon();
        let view = cx.new(|_: &mut Context<'_, Empty>| Empty);
        view.update(cx, |_, cx| perform(startup::Command::OpenLog, &link, cx));
        assert!(link.log().is_none(), "there was never a log to open");
    }
}
