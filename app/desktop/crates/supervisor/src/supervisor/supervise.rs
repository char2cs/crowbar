use std::sync::Arc;

use tokio::sync::mpsc;
use tokio_util::sync::CancellationToken;

use crate::supervisor::daemon_event::DaemonEvent;
use crate::supervisor::monitoring::Guard;
use crate::supervisor::monitoring::Running;
use crate::supervisor::monitoring::WatchContext;
use crate::supervisor::monitoring::refuse;
use crate::supervisor::monitoring::watch;
use crate::supervisor::monitoring::{monitor, stop_child};
use crate::supervisor::readiness::{await_ready, pid_mismatch};
use crate::supervisor::recovery::{idle, recover, stop_idle};
use crate::supervisor::spawn::spawn_daemon;
use crate::supervisor::supervision::Command;
use crate::supervisor::supervision::Phase;
use crate::supervisor::supervision::Startup;
use crate::supervisor::supervision::Supervision;

type Commands = mpsc::Receiver<Command>;

fn start_watchdog(
    supervision: &Supervision,
    child: tokio::process::Child,
) -> Running {
    let (wedged, wedged_rx) = mpsc::channel(1);
    let (healthy, healthy_rx) = mpsc::channel(1);
    let cancel = CancellationToken::new();
    tokio::spawn(watch(WatchContext {
        socket: supervision.config.paths.socket.clone(),
        pid: child.id(),
        timings: supervision.config.timings.clone(),
        bus: Arc::clone(&supervision.bus),
        wedged,
        healthy,
        cancel: cancel.clone(),
        clock: supervision.clock,
    }));
    Running {
        _watchdog: Guard::new(cancel),
        child,
        wedged: wedged_rx,
        healthy: healthy_rx,
    }
}

fn launch_event(launches: u32) -> DaemonEvent {
    if launches == 0 {
        return DaemonEvent::Starting;
    }
    DaemonEvent::Restarted
}

async fn launch(
    supervision: &mut Supervision,
    commands: &mut Commands,
) -> Phase {
    supervision.bus.emit(launch_event(supervision.launches));
    supervision.launches = supervision.launches.saturating_add(1);
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
            tracing::warn!("the daemon is not serving yet; it keeps running under the watchdog");
            monitor(supervision, start_watchdog(supervision, child), commands).await
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
    monitor(supervision, start_watchdog(supervision, child), commands).await
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
    use std::path::PathBuf;
    use std::time::Duration;

    use super::*;
    use crate::config::{Config, DaemonPaths};

    const NOWHERE: &str = "/crowbar-test-missing";

    fn missing_binary() -> Config {
        let paths = DaemonPaths {
            socket: PathBuf::from(NOWHERE).join("d.sock"),
            log: PathBuf::from(NOWHERE).join("d.log"),
        };
        let mut config = Config::new(
            PathBuf::from("/definitely/not/crowbar-api"),
            paths,
            None,
            "test",
        );
        config.timings.respawn_base = Duration::from_millis(1);
        config
    }

    async fn until_recovering(events: &mut tokio::sync::broadcast::Receiver<DaemonEvent>) {
        while !matches!(events.recv().await, Ok(DaemonEvent::Recovering { .. })) {}
    }

    #[tokio::test]
    async fn a_respawn_after_the_daemon_died_says_the_backend_restarted() {
        let supervisor = crate::Supervisor::start(missing_binary());
        let mut events = supervisor.events();
        until_recovering(&mut events).await;
        assert_eq!(events.recv().await, Ok(DaemonEvent::Restarted));
        supervisor.stop().await;
    }

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
