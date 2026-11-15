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
use crate::migration_progress::MigrationProgress;
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
use crate::watchdog_guard::WatchdogGuard;

type Commands = mpsc::Receiver<Command>;
type Raced = Result<(), Option<Command>>;

const DUMP_LIMIT: usize = 16 * 1024 * 1024;
const MAX_DOUBLINGS: u32 = 16;

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

fn recover_because(reason: &str) -> Phase {
    Phase::Recover {
        reason: reason.to_owned(),
    }
}

fn startup_of(
    outcome: Result<Option<u32>, Error>,
    spawned: Option<u32>,
) -> Startup {
    match outcome {
        Ok(pid) => Startup::Ready(pid.or(spawned)),
        Err(Error::Halted { reason, message }) => Startup::Halted { reason, message },
        Err(_) => Startup::NeverHealthy,
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
    child: tokio::process::Child,
    pid: Option<u32>,
    commands: &mut Commands,
) -> Phase {
    if let Some(warning) = pid_mismatch(pid, child.id()) {
        tracing::warn!("{warning}");
        supervision.note("===", &warning);
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

async fn stop_child(
    supervision: &Supervision,
    child: &mut tokio::process::Child,
) {
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
    let mut settled = None;
    while settled.is_none() {
        settled = watch_once(supervision, &mut running, commands).await;
    }
    settled.unwrap_or(Phase::Stop)
}

async fn watch_once(
    supervision: &mut Supervision,
    running: &mut Running,
    commands: &mut Commands,
) -> Option<Phase> {
    tokio::select! {
        status = running.child.wait() => Some(exited(supervision, &status)),
        command = commands.recv() => on_command(supervision, running, command).await,
        Some(()) = running.wedged.recv() => on_wedged(supervision, running, commands).await,
    }
}

fn exited(
    supervision: &Supervision,
    status: &std::io::Result<ExitStatus>,
) -> Phase {
    let reason = describe_exit(status);
    supervision.note("===", &reason);
    Phase::Recover { reason }
}

async fn on_command(
    supervision: &Supervision,
    running: &mut Running,
    command: Option<Command>,
) -> Option<Phase> {
    if let Some(Command::Restart(asked)) = command {
        reply(asked, Err(Error::AlreadyRunning));
        return None;
    }
    Some(stop_running(supervision, running, command).await)
}

async fn on_wedged(
    supervision: &Supervision,
    running: &mut Running,
    commands: &mut Commands,
) -> Option<Phase> {
    match kill_unless_stopped(supervision, running, commands).await {
        Ok(()) => None,
        Err(stop) => Some(stop_running(supervision, running, stop).await),
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
) -> Raced {
    let kill = std::pin::pin!(kill_wedged(supervision, &mut running.child));
    race_stop(kill, commands).await
}

async fn race_stop(
    mut kill: std::pin::Pin<&mut impl Future<Output = ()>>,
    commands: &mut Commands,
) -> Raced {
    let mut settled = None;
    while settled.is_none() {
        settled = race_once(&mut kill, commands).await;
    }
    settled.unwrap_or(Ok(()))
}

async fn race_once(
    kill: &mut std::pin::Pin<&mut impl Future<Output = ()>>,
    commands: &mut Commands,
) -> Option<Raced> {
    tokio::select! {
        () = &mut *kill => Some(Ok(())),
        command = commands.recv() => refuse_restart(command),
    }
}

fn refuse_restart(command: Option<Command>) -> Option<Raced> {
    if let Some(Command::Restart(asked)) = command {
        reply(asked, Err(Error::AlreadyRunning));
        return None;
    }
    Some(Err(command))
}

async fn kill_wedged(
    supervision: &Supervision,
    child: &mut tokio::process::Child,
) {
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

fn recovery_delay(
    base: Duration,
    cap: Duration,
    attempt: usize,
) -> Duration {
    let doublings = u32::try_from(attempt.saturating_sub(1))
        .unwrap_or(u32::MAX)
        .min(MAX_DOUBLINGS);
    (base * 2u32.saturating_pow(doublings)).min(cap)
}

async fn recover(
    supervision: &mut Supervision,
    reason: String,
    commands: &mut Commands,
) -> Phase {
    let t = supervision.config.timings.clone();
    match supervision.budget.on_death(Instant::now()) {
        Verdict::GiveUp { deaths } => give_up(supervision, deaths, t.restart_window).await,
        Verdict::Respawn { attempt } => {
            let delay = recovery_delay(t.respawn_base, t.respawn_cap, attempt);
            announce_recovery(supervision, attempt, delay, reason);
            backoff(supervision, delay, commands).await
        },
    }
}

async fn give_up(
    supervision: &Supervision,
    deaths: usize,
    window: Duration,
) -> Phase {
    let summary = crash_summary(deaths, window);
    supervision.flush_log().await;
    let report = crash_report(
        &summary,
        &supervision.config.paths.log,
        &supervision.config.version,
    );
    supervision.bus.emit(DaemonEvent::GaveUp { report });
    Phase::Idle
}

fn announce_recovery(
    supervision: &Supervision,
    attempt: usize,
    delay: Duration,
    reason: String,
) {
    let attempt = u32::try_from(attempt).unwrap_or(u32::MAX);
    supervision.bus.emit(DaemonEvent::Recovering {
        attempt,
        delay,
        reason,
    });
}

async fn backoff(
    supervision: &mut Supervision,
    delay: Duration,
    commands: &mut Commands,
) -> Phase {
    tokio::select! {
        () = tokio::time::sleep(delay) => Phase::Launch,
        command = commands.recv() => while_down(supervision, command),
    }
}

fn stop_idle(
    supervision: &Supervision,
    command: Option<Command>,
) -> Phase {
    supervision.bus.emit(DaemonEvent::Stopped);
    if let Some(Command::Stop(done)) = command {
        reply(done, ());
    }
    Phase::Stop
}

fn while_down(
    supervision: &mut Supervision,
    command: Option<Command>,
) -> Phase {
    let Some(Command::Restart(asked)) = command else {
        return stop_idle(supervision, command);
    };
    supervision.budget.reset();
    reply(asked, Ok(()));
    Phase::Launch
}

async fn idle(
    supervision: &mut Supervision,
    commands: &mut Commands,
) -> Phase {
    let command = commands.recv().await;
    while_down(supervision, command)
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
    fn respawn_delays_double_up_to_the_cap() {
        let base = Duration::from_millis(500);
        let cap = Duration::from_secs(10);
        let delays: Vec<Duration> = (1..=7).map(|n| recovery_delay(base, cap, n)).collect();
        assert_eq!(
            delays,
            [500, 1000, 2000, 4000, 8000, 10_000, 10_000].map(Duration::from_millis)
        );
        assert_eq!(recovery_delay(base, cap, 0), base);
        assert_eq!(recovery_delay(base, cap, usize::MAX), cap);
    }

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
}
