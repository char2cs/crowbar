use std::process::ExitStatus;

use tokio::sync::mpsc;

use crate::command::Command;
use crate::daemon_event::DaemonEvent;
use crate::error::Error;
use crate::health::wait_for_health;
use crate::migration_progress::MigrationProgress;
use crate::reply::reply;
use crate::startup::Startup;
use crate::supervision::Supervision;

type Commands = mpsc::Receiver<Command>;

pub(crate) fn describe_exit(status: &std::io::Result<ExitStatus>) -> String {
    match status {
        Ok(status) => format!("daemon terminated ({status})"),
        Err(e) => format!("daemon terminated (cannot read its status: {e})"),
    }
}

pub(crate) async fn await_ready(
    supervision: &Supervision,
    child: &mut tokio::process::Child,
    commands: &mut Commands,
) -> Startup {
    let bus = &supervision.bus;
    let health = wait_for_health(
        &supervision.config.paths.socket,
        &supervision.config.timings,
        |progress| bus.emit(migrating(progress)),
    );
    tokio::select! {
        outcome = health => startup_of(outcome, child.id()),
        status = child.wait() => Startup::Exited(describe_exit(&status)),
        command = commands.recv() => stopped_before_ready(command),
    }
}

fn migrating(progress: Option<&MigrationProgress>) -> DaemonEvent {
    DaemonEvent::Migrating {
        progress: progress.cloned(),
    }
}

pub(crate) fn startup_of(
    outcome: Result<Option<u32>, Error>,
    spawned: Option<u32>,
) -> Startup {
    match outcome {
        Ok(pid) => Startup::Ready(pid.or(spawned)),
        Err(Error::Halted { reason, message }) => Startup::Halted { reason, message },
        Err(failure) => {
            tracing::debug!("the daemon never became healthy: {failure}");
            Startup::NeverHealthy
        },
    }
}

fn stopped_before_ready(command: Option<Command>) -> Startup {
    answer_stop_or_busy(command);
    Startup::Stopped
}

fn answer_stop_or_busy(command: Option<Command>) {
    match command {
        Some(Command::Stop(done)) => reply(done, ()),
        Some(Command::Restart(asked)) => reply(asked, Err(Error::AlreadyRunning)),
        None => {},
    }
}

pub(crate) fn pid_mismatch(
    reported: Option<u32>,
    spawned: Option<u32>,
) -> Option<String> {
    match (reported, spawned) {
        (Some(reported), Some(spawned)) if reported != spawned => Some(format!(
            "daemon reports pid {reported} but the process we started is {spawned}: a stale daemon may own the socket"
        )),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_daemon_that_reports_another_pid_than_the_one_started_is_named_stale() {
        let warning = pid_mismatch(Some(99), Some(7)).unwrap();
        assert!(
            warning.contains("99") && warning.contains('7') && warning.contains("stale"),
            "{warning}"
        );
    }

    #[test]
    fn matching_or_unknown_pids_raise_no_warning() {
        let cases = [
            (Some(7), Some(7)),
            (None, Some(7)),
            (Some(7), None),
            (None, None),
        ];
        for (reported, spawned) in cases {
            assert_eq!(
                pid_mismatch(reported, spawned),
                None,
                "{reported:?} {spawned:?}"
            );
        }
    }

    #[test]
    fn a_halt_from_the_daemon_is_not_retried_and_other_failures_are_never_healthy() {
        let halted = startup_of(
            Err(Error::Halted {
                reason: Some("disk".into()),
                message: "full".into(),
            }),
            Some(7),
        );
        assert!(matches!(halted, Startup::Halted { .. }));
        assert!(matches!(
            startup_of(Ok(None), Some(7)),
            Startup::Ready(Some(7))
        ));
        assert!(matches!(
            startup_of(Err(Error::AlreadyRunning), Some(7)),
            Startup::NeverHealthy
        ));
    }
}
