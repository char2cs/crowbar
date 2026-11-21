use std::process::ExitStatus;

use tokio::sync::mpsc;

use crate::error::Error;
use crate::health::MigrationProgress;
use crate::health::wait_for_health;
use crate::supervisor::daemon_event::DaemonEvent;
use crate::supervisor::served_now::served_now;
use crate::supervisor::supervision::Command;
use crate::supervisor::supervision::Startup;
use crate::supervisor::supervision::Supervision;
use crate::supervisor::supervision::reply;

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
    let mut health = std::pin::pin!(health);
    let mut startup = None;
    while startup.is_none() {
        startup = startup_step(supervision, health.as_mut(), child, commands).await;
    }
    startup.unwrap_or(Startup::Stopped)
}

async fn startup_step(
    supervision: &Supervision,
    health: std::pin::Pin<&mut impl Future<Output = Result<Option<u32>, Error>>>,
    child: &mut tokio::process::Child,
    commands: &mut Commands,
) -> Option<Startup> {
    let spawned = child.id();
    tokio::select! {
        outcome = health => Some(startup_of(outcome, spawned)),
        status = child.wait() => Some(Startup::Exited(describe_exit(&status))),
        command = commands.recv() => during_startup(supervision, command, spawned).await,
    }
}

async fn during_startup(
    supervision: &Supervision,
    command: Option<Command>,
    spawned: Option<u32>,
) -> Option<Startup> {
    let Some(Command::Answered) = command else {
        return Some(stopped_before_ready(command));
    };
    let pid = served_now(supervision).await?;
    Some(Startup::Ready(pid.or(spawned)))
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
        Some(Command::Answered) | None => {},
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
