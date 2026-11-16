use std::sync::Arc;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::command::Command;
use crate::daemon_event::DaemonEvent;
use crate::monitoring::{monitor, stop_child};
use crate::phase::Phase;
use crate::readiness::{await_ready, pid_mismatch};
use crate::recovery::{idle, recover, stop_idle};
use crate::running::Running;
use crate::spawn_daemon::spawn_daemon;
use crate::stale_daemon::refuse;
use crate::startup::Startup;
use crate::supervision::Supervision;
use crate::watch_context::WatchContext;
use crate::watchdog::watch;
use crate::watchdog_guard::WatchdogGuard;

type Commands = mpsc::Receiver<Command>;

fn start_watchdog(
    supervision: &Supervision,
    pid: Option<u32>,
) -> (mpsc::Receiver<()>, CancellationToken) {
    let (wedged, wedged_rx) = mpsc::channel(1);
    let cancel = CancellationToken::new();
    tokio::spawn(watch(WatchContext {
        socket: supervision.config.paths.socket.clone(),
        pid,
        timings: supervision.config.timings.clone(),
        bus: Arc::clone(&supervision.bus),
        wedged,
        cancel: cancel.clone(),
        clock: supervision.clock,
    }));
    (wedged_rx, cancel)
}

fn recover_because(reason: &str) -> Phase {
    Phase::Recover {
        reason: reason.to_owned(),
    }
}

async fn launch(
    supervision: &mut Supervision,
    commands: &mut Commands,
) -> Phase {
    supervision.bus.emit(DaemonEvent::Starting);
    supervision.note("===", "daemon spawned");
    match spawn_daemon(supervision) {
        Ok(child) => supervise_child(supervision, child, commands).await,
        Err(e) => Phase::Recover {
            reason: e.to_string(),
        },
    }
}

async fn supervise_child(
    supervision: &mut Supervision,
    mut child: tokio::process::Child,
    commands: &mut Commands,
) -> Phase {
    let startup = await_ready(supervision, &mut child, commands).await;
    after_startup(supervision, child, startup, commands).await
}

async fn after_startup(
    supervision: &mut Supervision,
    mut child: tokio::process::Child,
    startup: Startup,
    commands: &mut Commands,
) -> Phase {
    match startup {
        Startup::Ready(pid) => become_ready(supervision, child, pid, commands).await,
        Startup::Halted { reason, message } => {
            stop_child(supervision, &mut child).await;
            halt(supervision, reason, message)
        },
        Startup::NeverHealthy => {
            stop_child(supervision, &mut child).await;
            recover_because("the daemon never became healthy")
        },
        Startup::Exited(reason) => Phase::Recover { reason },
        Startup::Stopped => {
            stop_child(supervision, &mut child).await;
            stop_idle(supervision, None)
        },
    }
}

fn halt(
    supervision: &Supervision,
    reason: Option<String>,
    message: String,
) -> Phase {
    supervision
        .bus
        .emit(DaemonEvent::Halted { reason, message });
    Phase::Idle
}

async fn become_ready(
    supervision: &mut Supervision,
    mut child: tokio::process::Child,
    pid: Option<u32>,
    commands: &mut Commands,
) -> Phase {
    if let Some(reason) = pid_mismatch(pid, child.id()) {
        return refuse(supervision, &mut child, reason).await;
    }
    supervision.bus.emit(DaemonEvent::Ready { pid });
    let (wedged, watchdog) = start_watchdog(supervision, child.id());
    let running = Running {
        _watchdog: WatchdogGuard::new(watchdog),
        child,
        wedged,
    };
    monitor(supervision, running, commands).await
}

async fn step(
    supervision: &mut Supervision,
    phase: Phase,
    commands: &mut Commands,
) -> Phase {
    match phase {
        Phase::Launch => launch(supervision, commands).await,
        Phase::Recover { reason } => recover(supervision, reason, commands).await,
        Phase::Idle => idle(supervision, commands).await,
        Phase::Stop => Phase::Stop,
    }
}

pub(crate) async fn supervise(
    mut supervision: Supervision,
    mut commands: Commands,
) {
    let mut phase = Phase::Launch;
    while !matches!(phase, Phase::Stop) {
        phase = step(&mut supervision, phase, &mut commands).await;
    }
    supervision.flush_log().await;
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_daemon_that_halts_leaves_the_supervisor_idle_and_says_why() {
        let supervision = crate::test_support::supervision();
        let mut events = supervision.bus.subscribe();
        let phase = halt(&supervision, Some("disk".into()), "full".into());
        assert!(matches!(phase, Phase::Idle));
        assert!(matches!(
            events.try_recv().unwrap(),
            DaemonEvent::Halted { .. }
        ));
    }
}
