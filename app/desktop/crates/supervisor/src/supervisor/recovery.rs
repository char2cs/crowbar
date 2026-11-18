use std::time::Duration;

use tokio::sync::mpsc;

use crate::supervisor::daemon_event::DaemonEvent;
use crate::supervisor::supervision::Command;
use crate::supervisor::supervision::Phase;
use crate::supervisor::supervision::Supervision;
use crate::supervisor::supervision::Verdict;
use crate::supervisor::supervision::reply;
use crate::supervisor::supervision::{crash_report, crash_summary};

type Commands = mpsc::Receiver<Command>;

const MAX_DOUBLINGS: u32 = 16;

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

pub(crate) async fn recover(
    supervision: &mut Supervision,
    reason: String,
    commands: &mut Commands,
) -> Phase {
    let t = supervision.config.timings.clone();
    match supervision.budget.on_death((supervision.clock.instant)()) {
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

pub(crate) fn stop_idle(
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

pub(crate) async fn idle(
    supervision: &mut Supervision,
    commands: &mut Commands,
) -> Phase {
    let command = commands.recv().await;
    while_down(supervision, command)
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
}
