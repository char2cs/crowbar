use std::time::{Duration, SystemTime};

use tokio::time::Instant;

use crate::daemon_event::DaemonEvent;
use crate::probe::{Probe, probe_ready};
use crate::process_cpu::{cpu_time, is_suspended};
use crate::watch_context::WatchContext;
use crate::watch_state::WatchState;
use crate::exoneration::Exoneration;
use crate::wedge::{cycle_was_suspended, made_progress, should_kill_wedged};

async fn probe_within(ctx: &WatchContext, budget: Duration) -> Probe {
    tokio::time::timeout(budget, probe_ready(&ctx.socket))
        .await
        .unwrap_or(Probe::Unserved)
}

async fn daemon_cpu(ctx: &WatchContext) -> Option<Duration> {
    cpu_time(ctx.pid?).await
}

fn nap_or_sleep_interrupted(ctx: &WatchContext, state: &mut WatchState) -> bool {
    let now = SystemTime::now();
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

async fn confirmed_wedge(ctx: &WatchContext, state: &mut WatchState) -> bool {
    let suspended = match ctx.pid {
        Some(pid) => is_suspended(pid).await,
        None => false,
    };
    let exonerated =
        !suspended && probe_within(ctx, ctx.timings.grace_probe_timeout).await != Probe::Unserved;
    let (silent_at, before) = state.silent_since.take().unwrap_or((Instant::now(), None));
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

async fn cycle(ctx: &WatchContext, state: &mut WatchState) -> bool {
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
    if outcome != Probe::Unserved {
        state.silent_since = None;
    } else if state.silent_since.is_none() {
        state.silent_since = Some((Instant::now(), daemon_cpu(ctx).await));
    }
    state.tracker.observe(outcome) && confirmed_wedge(ctx, state).await
}

pub(crate) async fn watch(ctx: WatchContext) {
    let mut state = WatchState::new(ctx.timings.failure_threshold);
    loop {
        tokio::select! {
            () = ctx.cancel.cancelled() => return,
            () = tokio::time::sleep(ctx.timings.watchdog_interval) => {}
        }
        if cycle(&ctx, &mut state).await {
            ctx.bus.emit(DaemonEvent::Unhealthy);
            ctx.wedged.send(()).await.ok();
            state.forget();
        }
    }
}

#[cfg(test)]
#[path = "watchdog_tests.rs"]
mod tests;
