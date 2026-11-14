use std::path::Path;
use std::process::ExitStatus;
use std::sync::Arc;
use std::time::Duration;

use rustix::process::Signal;
use tokio::sync::mpsc;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::command::Command;
use crate::crash_report::{crash_report, crash_summary};
use crate::daemon_event::DaemonEvent;
use crate::error::Error;
use crate::health::{get, wait_for_health};
use crate::phase::Phase;
use crate::running::Running;
use crate::spawn_daemon::spawn_daemon;
use crate::startup::Startup;
use crate::stop::{send, stop_with};
use crate::supervision::Supervision;
use crate::verdict::Verdict;
use crate::watch_context::WatchContext;
use crate::watchdog::watch;

type Commands = mpsc::Receiver<Command>;

const DUMP_LIMIT: usize = 16 * 1024 * 1024;

fn describe_exit(status: &std::io::Result<ExitStatus>) -> String {
    match status {
        Ok(status) => format!("daemon terminated ({status})"),
        Err(e) => format!("daemon terminated (cannot read its status: {e})"),
    }
}

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
    }));
    (wedged_rx, cancel)
}

async fn await_ready(
    supervision: &Supervision,
    child: &mut tokio::process::Child,
    commands: &mut Commands,
) -> Startup {
    let t = &supervision.config.timings;
    let health = wait_for_health(
        &supervision.config.paths.socket,
        t.health_attempts,
        t.health_poll,
        t.health_probe_timeout,
    );
    tokio::select! {
        outcome = health => match outcome {
            Ok(pid) => Startup::Ready(pid.or(child.id())),
            Err(Error::Halted(message)) => Startup::Halted(message),
            Err(_) => Startup::NeverHealthy,
        },
        status = child.wait() => Startup::Exited(describe_exit(&status)),
        command = commands.recv() => {
            answer_stop_or_busy(command);
            Startup::Stopped
        }
    }
}

fn answer_stop_or_busy(command: Option<Command>) {
    match command {
        Some(Command::Stop(done)) => {
            done.send(()).ok();
        },
        Some(Command::Restart(reply)) => {
            reply.send(Err(Error::AlreadyRunning)).ok();
        },
        None => {},
    }
}

async fn launch(supervision: &mut Supervision, commands: &mut Commands) -> Phase {
    supervision.bus.emit(DaemonEvent::Starting);
    supervision.note("===", "daemon spawned");
    let mut child = match spawn_daemon(supervision) {
        Ok(child) => child,
        Err(e) => {
            return Phase::Recover {
                reason: e.to_string(),
            };
        },
    };
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
        Startup::Ready(pid) => {
            supervision.bus.emit(DaemonEvent::Ready { pid });
            let (wedged, watchdog) = start_watchdog(supervision, child.id());
            let running = Running {
                child,
                wedged,
                watchdog,
            };
            monitor(supervision, running, commands).await
        },
        Startup::Halted(message) => {
            stop_child(supervision, &mut child).await;
            supervision.bus.emit(DaemonEvent::Halted { message });
            Phase::Idle
        },
        Startup::NeverHealthy => {
            stop_child(supervision, &mut child).await;
            Phase::Recover {
                reason: "the daemon never became healthy".to_owned(),
            }
        },
        Startup::Exited(reason) => Phase::Recover { reason },
        Startup::Stopped => {
            stop_child(supervision, &mut child).await;
            supervision.bus.emit(DaemonEvent::Stopped);
            Phase::Stop
        },
    }
}

async fn stop_child(supervision: &Supervision, child: &mut tokio::process::Child) {
    let status = stop_with(
        child,
        Signal::TERM,
        supervision.config.timings.sigterm_grace,
    )
    .await;
    supervision.note("===", &describe_exit(&status));
}

async fn monitor(
    supervision: &mut Supervision,
    mut running: Running,
    commands: &mut Commands,
) -> Phase {
    loop {
        tokio::select! {
            status = running.child.wait() => {
                let reason = describe_exit(&status);
                supervision.note("===", &reason);
                return Phase::Recover { reason };
            }
            command = commands.recv() => match command {
                Some(Command::Restart(reply)) => { reply.send(Err(Error::AlreadyRunning)).ok(); }
                other => return stop_running(supervision, &mut running, other).await,
            },
            Some(()) = running.wedged.recv() => kill_wedged(supervision, &mut running).await,
        }
    }
}

async fn stop_running(
    supervision: &Supervision,
    running: &mut Running,
    command: Option<Command>,
) -> Phase {
    stop_child(supervision, &mut running.child).await;
    supervision.bus.emit(DaemonEvent::Stopped);
    if let Some(Command::Stop(done)) = command {
        done.send(()).ok();
    }
    Phase::Stop
}

async fn kill_wedged(supervision: &Supervision, running: &mut Running) {
    let t = &supervision.config.timings;
    let socket: &Path = &supervision.config.paths.socket;
    let dump = tokio::time::timeout(
        t.dump_timeout,
        get(socket, "/debug/pprof/goroutine?debug=2", DUMP_LIMIT),
    )
    .await;
    if let Ok(Ok(body)) = dump {
        supervision.note(
            "=== watchdog goroutine dump",
            &String::from_utf8_lossy(&body),
        );
    }
    send(&running.child, Signal::QUIT);
    if tokio::time::timeout(t.sigquit_grace, running.child.wait())
        .await
        .is_err()
    {
        running.child.kill().await.ok();
    }
}

fn recovery_delay(base: Duration, cap: Duration, attempt: usize) -> Duration {
    let doublings = u32::try_from(attempt.saturating_sub(1))
        .unwrap_or(u32::MAX)
        .min(16);
    (base * 2u32.saturating_pow(doublings)).min(cap)
}

async fn recover(supervision: &mut Supervision, reason: String, commands: &mut Commands) -> Phase {
    let t = supervision.config.timings.clone();
    match supervision.budget.on_death(Instant::now()) {
        Verdict::GiveUp { deaths } => {
            let summary = crash_summary(deaths, t.restart_window);
            let report = crash_report(&summary, &supervision.config.paths.log);
            supervision.bus.emit(DaemonEvent::GaveUp { report });
            Phase::Idle
        },
        Verdict::Respawn { attempt } => {
            let delay = recovery_delay(t.respawn_base, t.respawn_cap, attempt);
            let attempt = u32::try_from(attempt).unwrap_or(u32::MAX);
            supervision.bus.emit(DaemonEvent::Recovering {
                attempt,
                delay,
                reason,
            });
            backoff(supervision, delay, commands).await
        },
    }
}

async fn backoff(supervision: &mut Supervision, delay: Duration, commands: &mut Commands) -> Phase {
    tokio::select! {
        () = tokio::time::sleep(delay) => Phase::Launch,
        command = commands.recv() => match command {
            Some(Command::Restart(reply)) => {
                supervision.budget.reset();
                reply.send(Ok(())).ok();
                Phase::Launch
            }
            other => stop_idle(supervision, other),
        },
    }
}

fn stop_idle(supervision: &Supervision, command: Option<Command>) -> Phase {
    supervision.bus.emit(DaemonEvent::Stopped);
    if let Some(Command::Stop(done)) = command {
        done.send(()).ok();
    }
    Phase::Stop
}

async fn idle(supervision: &mut Supervision, commands: &mut Commands) -> Phase {
    match commands.recv().await {
        Some(Command::Restart(reply)) => {
            supervision.budget.reset();
            reply.send(Ok(())).ok();
            Phase::Launch
        },
        other => stop_idle(supervision, other),
    }
}

pub(crate) async fn supervise(mut supervision: Supervision, mut commands: Commands) {
    let mut phase = Phase::Launch;
    loop {
        phase = match phase {
            Phase::Launch => launch(&mut supervision, &mut commands).await,
            Phase::Recover { reason } => recover(&mut supervision, reason, &mut commands).await,
            Phase::Idle => idle(&mut supervision, &mut commands).await,
            Phase::Stop => return,
        };
    }
}

#[cfg(test)]
#[path = "supervise_tests.rs"]
mod tests;
