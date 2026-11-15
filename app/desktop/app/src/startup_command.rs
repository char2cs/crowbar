use gpui::Context;
use startup_view::StartupCommand;

use crate::daemon_link::DaemonLink;

/// Runs a button of the startup view.
pub(crate) fn perform<V: 'static>(
    command: StartupCommand,
    link: &DaemonLink,
    cx: &mut Context<'_, V>,
) {
    match command {
        StartupCommand::Retry => link.restart(),
        StartupCommand::OpenLog => open_log(link),
        StartupCommand::Quit => cx.quit(),
        _ => {},
    }
}

fn open_log(link: &DaemonLink) {
    let Some(log) = link.log() else {
        tracing::warn!("there is no daemon log to open");
        return;
    };
    let opener = if cfg!(target_os = "macos") {
        "open"
    } else {
        "xdg-open"
    };
    if let Err(error) = std::process::Command::new(opener).arg(log).spawn() {
        tracing::warn!(%error, "opening the daemon log failed");
    }
}
