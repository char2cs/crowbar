use gpui::Context;
use startup_view::StartupCommand;

use crate::daemon_link::DaemonLink;
use crate::startup_action::StartupAction;

#[cfg(target_os = "macos")]
const OPENER: &str = "open";
#[cfg(not(target_os = "macos"))]
const OPENER: &str = "xdg-open";

pub(crate) fn perform<V: 'static>(
    command: StartupCommand,
    link: &DaemonLink,
    cx: &mut Context<'_, V>,
) {
    match action_for(command) {
        StartupAction::Relaunch => link.restart(),
        StartupAction::ShowLog => open_log(link),
        StartupAction::QuitApp => cx.quit(),
        StartupAction::Nothing => {},
    }
}

fn action_for(command: StartupCommand) -> StartupAction {
    match command {
        StartupCommand::Retry => StartupAction::Relaunch,
        StartupCommand::OpenLog => StartupAction::ShowLog,
        StartupCommand::Quit => StartupAction::QuitApp,
        _ => StartupAction::Nothing,
    }
}

fn open_log(link: &DaemonLink) {
    let Some(log) = link.log() else {
        tracing::warn!("there is no daemon log to open");
        return;
    };
    if let Err(error) = std::process::Command::new(OPENER).arg(&log).spawn() {
        tracing::warn!(%error, "opening the daemon log failed");
    }
}

#[cfg(test)]
mod tests {
    use gpui::{AppContext as _, Context, TestAppContext};

    use super::*;
    use crate::app_options::AppOptions;

    struct Empty;

    fn link() -> (DaemonLink, tempfile::TempDir) {
        let home = tempfile::tempdir().unwrap();
        let options = AppOptions {
            panes_dir: None,
            chat_entry: "chat/index.html".into(),
            home: home.path().to_path_buf(),
            daemon_binary: Some(home.path().join("missing-daemon")),
        };
        (DaemonLink::start(&options).unwrap(), home)
    }

    #[test]
    fn each_command_maps_to_its_action() {
        const CASES: [(StartupCommand, StartupAction); 3] = [
            (StartupCommand::Retry, StartupAction::Relaunch),
            (StartupCommand::OpenLog, StartupAction::ShowLog),
            (StartupCommand::Quit, StartupAction::QuitApp),
        ];
        for (command, action) in CASES {
            assert_eq!(action_for(command), action);
        }
    }

    #[gpui::test]
    fn retry_after_a_failed_launch_reports_a_new_attempt(cx: &mut TestAppContext) {
        let (link, _home) = link();
        let phases = link.phases();
        while phases.try_recv().is_ok() {}
        let view = cx.new(|_: &mut Context<'_, Empty>| Empty);
        view.update(cx, |_, cx| perform(StartupCommand::Retry, &link, cx));
        assert!(
            phases.recv_blocking().is_ok(),
            "the outage screen hears of it"
        );
    }

    #[gpui::test]
    fn opening_a_log_that_does_not_exist_is_harmless(cx: &mut TestAppContext) {
        let (link, _home) = link();
        let view = cx.new(|_: &mut Context<'_, Empty>| Empty);
        view.update(cx, |_, cx| perform(StartupCommand::OpenLog, &link, cx));
        assert!(link.log().is_none(), "there was never a log to open");
    }
}
