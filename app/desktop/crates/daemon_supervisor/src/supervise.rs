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
use crate::daemon_get::get;
use crate::error::Error;
use crate::health::wait_for_health;
use crate::phase::Phase;
use crate::reply::reply;
use crate::running::Running;
use crate::socket_dial::SocketDial;
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
    let bus = &supervision.bus;
    let health = wait_for_health(
        &supervision.config.paths.socket,
        &supervision.config.timings,
        |progress| {
            bus.emit(DaemonEvent::Migrating {
                progress: progress.cloned(),
            });
        },
    );
    tokio::select! {
        outcome = health => match outcome {
            Ok(pid) => Startup::Ready(pid.or(child.id())),
            Err(Error::Halted { reason, message }) => Startup::Halted { reason, message },
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
        Some(Command::Stop(done)) => reply(done, ()),
        Some(Command::Restart(asked)) => reply(asked, Err(Error::AlreadyRunning)),
        None => {},
    }
}

pub(crate) fn pid_mismatch(reported: Option<u32>, spawned: Option<u32>) -> Option<String> {
    match (reported, spawned) {
        (Some(reported), Some(spawned)) if reported != spawned => Some(format!(
            "daemon reports pid {reported} but the process we started is {spawned}: a stale daemon may own the socket"
        )),
        _ => None,
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
            if let Some(warning) = pid_mismatch(pid, child.id()) {
                tracing::warn!("{warning}");
                supervision.note("===", &warning);
            }
            supervision.bus.emit(DaemonEvent::Ready { pid });
            let (wedged, watchdog) = start_watchdog(supervision, child.id());
            let running = Running {
                child,
                wedged,
                watchdog,
            };
            monitor(supervision, running, commands).await
        },
        Startup::Halted { reason, message } => {
            stop_child(supervision, &mut child).await;
            supervision
                .bus
                .emit(DaemonEvent::Halted { reason, message });
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
                Some(Command::Restart(asked)) => reply(asked, Err(Error::AlreadyRunning)),
                other => return stop_running(supervision, &mut running, other).await,
            },
            Some(()) = running.wedged.recv() => {
                if let Err(stop) = kill_unless_stopped(supervision, &mut running, commands).await {
                    return stop_running(supervision, &mut running, stop).await;
                }
            }
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
        reply(done, ());
    }
    Phase::Stop
}

async fn kill_unless_stopped(
    supervision: &Supervision,
    running: &mut Running,
    commands: &mut Commands,
) -> Result<(), Option<Command>> {
    let kill = std::pin::pin!(kill_wedged(supervision, &mut running.child));
    race_stop(kill, commands).await
}

async fn race_stop(
    mut kill: std::pin::Pin<&mut impl Future<Output = ()>>,
    commands: &mut Commands,
) -> Result<(), Option<Command>> {
    loop {
        tokio::select! {
            () = &mut kill => return Ok(()),
            command = commands.recv() => match command {
                Some(Command::Restart(asked)) => reply(asked, Err(Error::AlreadyRunning)),
                other => return Err(other),
            },
        }
    }
}

async fn kill_wedged(supervision: &Supervision, child: &mut tokio::process::Child) {
    let t = &supervision.config.timings;
    let dump = tokio::time::timeout(
        t.dump_timeout,
        get(
            &SocketDial::new(&supervision.config.paths.socket),
            "/debug/pprof/goroutine?debug=2",
            DUMP_LIMIT,
        ),
    )
    .await;
    match dump {
        Ok(Ok(body)) => supervision.note(
            "=== watchdog goroutine dump",
            &String::from_utf8_lossy(&body),
        ),
        Ok(Err(e)) => tracing::debug!("no goroutine dump: {e}"),
        Err(_) => tracing::debug!("no goroutine dump within {:?}", t.dump_timeout),
    }
    send(child, Signal::QUIT);
    if tokio::time::timeout(t.sigquit_grace, child.wait())
        .await
        .is_err()
        && let Err(e) = child.kill().await
    {
        tracing::warn!("cannot kill the wedged daemon: {e}");
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
            supervision.flush_log().await;
            let report = crash_report(
                &summary,
                &supervision.config.paths.log,
                &supervision.config.version,
            );
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
            Some(Command::Restart(asked)) => {
                supervision.budget.reset();
                reply(asked, Ok(()));
                Phase::Launch
            }
            other => stop_idle(supervision, other),
        },
    }
}

fn stop_idle(supervision: &Supervision, command: Option<Command>) -> Phase {
    supervision.bus.emit(DaemonEvent::Stopped);
    if let Some(Command::Stop(done)) = command {
        reply(done, ());
    }
    Phase::Stop
}

async fn idle(supervision: &mut Supervision, commands: &mut Commands) -> Phase {
    match commands.recv().await {
        Some(Command::Restart(asked)) => {
            supervision.budget.reset();
            reply(asked, Ok(()));
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
            Phase::Stop => {
                supervision.flush_log().await;
                return;
            },
        };
    }
}

#[cfg(test)]
#[path = "supervise_tests.rs"]
mod tests;
