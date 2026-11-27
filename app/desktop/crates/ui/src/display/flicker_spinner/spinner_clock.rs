use std::time::{Duration, Instant};

use gpui::{App, EntityId, Global, Task};

pub(crate) const STEP: Duration = Duration::from_millis(90);

pub(crate) struct SpinnerClock {
    epoch: Instant,
    waiting: Vec<EntityId>,
    tick: Option<Task<()>>,
}

impl Global for SpinnerClock {}

pub(crate) fn aligned_now(cx: &mut App) -> Instant {
    let now = cx.background_executor().now();
    let epoch = epoch(cx);
    now.checked_sub(into_step(now - epoch)).unwrap_or(now)
}

pub(crate) fn wake(
    playback: EntityId,
    cx: &mut App,
) {
    let now = cx.background_executor().now();
    let epoch = epoch(cx);
    let clock = cx.global_mut::<SpinnerClock>();
    if !clock.waiting.contains(&playback) {
        clock.waiting.push(playback);
    }
    if clock.tick.is_some() {
        return;
    }
    let left = STEP.saturating_sub(into_step(now - epoch));
    let tick = cx.spawn(async move |cx| {
        cx.background_executor().timer(left).await;
        cx.update(tick);
    });
    cx.global_mut::<SpinnerClock>().tick = Some(tick);
}

fn epoch(cx: &mut App) -> Instant {
    if let Some(clock) = cx.try_global::<SpinnerClock>() {
        return clock.epoch;
    }
    let epoch = cx.background_executor().now();
    cx.set_global(SpinnerClock {
        epoch,
        waiting: Vec::new(),
        tick: None,
    });
    epoch
}

fn into_step(elapsed: Duration) -> Duration {
    let step = STEP.as_nanos();
    Duration::from_nanos(u64::try_from(elapsed.as_nanos() % step).unwrap_or_default())
}

fn tick(cx: &mut App) {
    let clock = cx.global_mut::<SpinnerClock>();
    clock.tick = None;
    let waiting = std::mem::take(&mut clock.waiting);
    for playback in waiting {
        cx.notify(playback);
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_time_inside_a_step_reads_how_far_past_its_start_it_is() {
        assert_eq!(into_step(Duration::ZERO), Duration::ZERO);
        assert_eq!(into_step(STEP * 3 + STEP / 3), STEP / 3);
    }
}
