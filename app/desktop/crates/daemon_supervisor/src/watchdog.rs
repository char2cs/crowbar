use std::time::Duration;

use crate::daemon_event::DaemonEvent;
use crate::exoneration::Exoneration;
use crate::probe::{Probe, probe_ready};
use crate::process_cpu::{cpu_time, is_suspended};
use crate::watch_context::WatchContext;
use crate::watch_state::WatchState;
use crate::wedge::{cycle_was_suspended, made_progress, should_kill_wedged};

async fn probe_within(
    ctx: &WatchContext,
    budget: Duration,
) -> Probe {
    tokio::time::timeout(budget, probe_ready(&ctx.socket))
        .await
        .unwrap_or(Probe::Unserved)
}

async fn daemon_cpu(ctx: &WatchContext) -> Option<Duration> {
    cpu_time(ctx.pid?).await
}

fn nap_or_sleep_interrupted(
    ctx: &WatchContext,
    state: &mut WatchState,
) -> bool {
    let now = (ctx.clock.wall)();
    let cycle = now
        .duration_since(state.previous_cycle)
        .unwrap_or(Duration::ZERO);
    state.previous_cycle = now;
    let t = &ctx.timings;
    cycle_was_suspended(
        cycle,
        t.watchdog_interval,
        t.probe_timeout,
        t.suspension_margin,
    )
}

async fn confirmed_wedge(
    ctx: &WatchContext,
    state: &mut WatchState,
) -> bool {
    let suspended = match ctx.pid {
        Some(pid) => is_suspended(pid).await,
        None => false,
    };
    let exonerated =
        !suspended && probe_within(ctx, ctx.timings.grace_probe_timeout).await != Probe::Unserved;
    let (silent_at, before) = state
        .silent_since
        .take()
        .unwrap_or(((ctx.clock.instant)(), None));
    let working = made_progress(before, daemon_cpu(ctx).await, silent_at.elapsed());
    let mut exonerations = Vec::new();
    if suspended {
        exonerations.push(Exoneration::Suspended);
    }
    if exonerated {
        exonerations.push(Exoneration::Answering);
    }
    if working {
        exonerations.push(Exoneration::Busy);
    }
    if should_kill_wedged(&exonerations) {
        return true;
    }
    tracing::warn!(
        "daemon failed its probes but is not wedged (suspended: {suspended}, answering: {exonerated}, busy: {working})"
    );
    state.tracker.reset();
    false
}

async fn track_silence(
    ctx: &WatchContext,
    state: &mut WatchState,
    outcome: Probe,
) {
    if outcome != Probe::Unserved {
        state.silent_since = None;
        return;
    }
    if state.silent_since.is_none() {
        state.silent_since = Some(((ctx.clock.instant)(), daemon_cpu(ctx).await));
    }
}

async fn cycle(
    ctx: &WatchContext,
    state: &mut WatchState,
) -> bool {
    if nap_or_sleep_interrupted(ctx, state) {
        state.forget();
        return false;
    }
    let outcome = probe_within(ctx, ctx.timings.probe_timeout).await;
    if outcome == Probe::LocalDescriptorExhaustion {
        tracing::error!("out of file descriptors, cannot dial the daemon; not restarting it");
        state.forget();
        return false;
    }
    track_silence(ctx, state, outcome).await;
    state.tracker.observe(outcome) && confirmed_wedge(ctx, state).await
}

pub(crate) async fn watch(ctx: WatchContext) {
    let mut state = WatchState::new(ctx.timings.failure_threshold, (ctx.clock.wall)());
    while watch_once(&ctx, &mut state).await {}
}

async fn watch_once(
    ctx: &WatchContext,
    state: &mut WatchState,
) -> bool {
    tokio::select! {
        () = ctx.cancel.cancelled() => return false,
        () = tokio::time::sleep(ctx.timings.watchdog_interval) => {}
    }
    let wedged = tokio::select! {
        () = ctx.cancel.cancelled() => return false,
        wedged = cycle(ctx, state) => wedged,
    };
    if wedged {
        announce_wedge(ctx).await;
        state.forget();
    }
    true
}

async fn announce_wedge(ctx: &WatchContext) {
    if ctx.cancel.is_cancelled() {
        return;
    }
    ctx.bus.emit(DaemonEvent::Unhealthy);
    if ctx.wedged.send(()).await.is_err() {
        tracing::debug!("the supervisor stopped listening for wedge reports");
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::adapters::system_clock;
    use crate::bus::Bus;
    use crate::timings::Timings;
    use std::sync::Arc;
    use tokio::sync::mpsc;
    use tokio_util::sync::CancellationToken;

    fn context(
        socket: std::path::PathBuf,
        cancel: CancellationToken,
        wedged: mpsc::Sender<()>,
    ) -> WatchContext {
        let timings = Timings {
            watchdog_interval: Duration::from_millis(1),
            probe_timeout: Duration::from_millis(50),
            grace_probe_timeout: Duration::from_millis(50),
            failure_threshold: 2,
            ..Timings::default()
        };
        WatchContext {
            socket,
            pid: None,
            timings,
            bus: Arc::new(Bus::default()),
            wedged,
            cancel,
            clock: system_clock(),
        }
    }

    #[tokio::test]
    async fn cancelling_ends_the_watch() {
        let (wedged, _rx) = mpsc::channel(1);
        let cancel = CancellationToken::new();
        cancel.cancel();
        watch(context("/nope.sock".into(), cancel, wedged)).await;
    }

    #[tokio::test]
    async fn a_daemon_that_cannot_be_dialled_is_reported_wedged_after_the_threshold_with_no_pid_to_exonerate_it()
     {
        let (wedged, mut asked) = mpsc::channel(1);
        let cancel = CancellationToken::new();
        let ctx = context("/definitely/absent.sock".into(), cancel.clone(), wedged);
        let bus = Arc::clone(&ctx.bus);
        let mut events = bus.subscribe();
        let task = tokio::spawn(watch(ctx));
        tokio::time::timeout(Duration::from_secs(30), asked.recv())
            .await
            .unwrap()
            .unwrap();
        assert_eq!(events.recv().await.unwrap(), DaemonEvent::Unhealthy);
        cancel.cancel();
        task.await.unwrap();
    }

    #[ignore = "real-infra: holds real unix sockets open without answering"]
    async fn hold_connections_open(listener: tokio::net::UnixListener) {
        let mut open = Vec::new();
        while let Ok((stream, _)) = listener.accept().await {
            open.push(stream);
        }
    }

    #[tokio::test(start_paused = true)]
    #[ignore = "real-infra: holds real unix sockets open without answering"]
    async fn a_cycle_that_is_still_probing_when_the_watch_is_cancelled_reports_nothing() {
        let dir = tempfile::tempdir().unwrap();
        let socket = dir.path().join("silent.sock");
        let listener = tokio::net::UnixListener::bind(&socket).unwrap();
        let held = tokio::spawn(hold_connections_open(listener));
        let (wedged, mut asked) = mpsc::channel(1);
        let cancel = CancellationToken::new();
        let mut ctx = context(socket, cancel.clone(), wedged);
        ctx.timings.failure_threshold = 1;
        ctx.timings.watchdog_interval = Duration::from_secs(1);
        ctx.timings.probe_timeout = Duration::from_secs(5);
        ctx.timings.grace_probe_timeout = Duration::from_secs(5);
        let mut events = ctx.bus.subscribe();
        let task = tokio::spawn(watch(ctx));
        tokio::time::sleep(Duration::from_secs(3)).await;
        cancel.cancel();
        task.await.unwrap();
        tokio::time::sleep(Duration::from_secs(60)).await;
        assert!(
            events.try_recv().is_err(),
            "no Unhealthy for a daemon that is gone"
        );
        assert!(asked.try_recv().is_err(), "no kill request either");
        held.abort();
    }
}
