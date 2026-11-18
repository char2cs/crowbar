//! The inputs each phase of the startup gallery feeds the view.

use std::time::Duration;

use startup::{MigrationStep, Outage, StartupInput, StartupTiming};

pub(crate) const STEPS: u32 = 5;
pub(crate) const DEMO_PHASE: &str = "demo";
pub(crate) const REPORT: &str = concat!(
    "crowbar 0.1.0 linux x86_64\n",
    "the daemon exited 3 times in 60s\n\n",
    "--- log ---\n",
    "2026-11-14T09:12:01Z INFO  opening home\n",
    "2026-11-14T09:12:01Z ERROR migrate step 3 failed: no such table: spaces\n",
    "2026-11-14T09:12:02Z INFO  shutting down",
);
pub(crate) const HALT_MESSAGE: &str =
    "migrate: step 3 \"Convert workspace settings\": no such table: spaces";
pub(crate) const RESTART_ATTEMPT: u32 = 2;
pub(crate) const MIGRATING_PHASE_STEP: u32 = 2;

pub(crate) fn migrating(index: u32) -> StartupInput {
    let names = [
        "Move chats to the new layout",
        "Rebuild the search index",
        "Convert workspace settings",
        "Backfill review threads",
        "Verify the result",
    ];
    let name = names
        .get(usize::try_from(index.saturating_sub(1)).unwrap_or(0))
        .copied()
        .unwrap_or("Finish");
    StartupInput::Migrating {
        step: Some(MigrationStep::new(name, index, STEPS)),
    }
}

pub(crate) fn halted() -> StartupInput {
    StartupInput::Halted {
        reason: Some("step_failed".to_owned()),
        message: HALT_MESSAGE.to_owned(),
    }
}

pub(crate) fn restarting() -> StartupInput {
    StartupInput::Unavailable(Outage::Recovering {
        attempt: RESTART_ATTEMPT,
        reason: "the daemon exited with status 1".to_owned(),
    })
}

pub(crate) fn phase_input(phase: &str) -> Vec<StartupInput> {
    match phase {
        "starting" => vec![StartupInput::Starting],
        "migrating" => vec![migrating(MIGRATING_PHASE_STEP)],
        "halted" => vec![halted()],
        "restarting" => vec![restarting()],
        "unresponsive" => vec![StartupInput::Unavailable(Outage::Unresponsive)],
        "gave-up" => vec![StartupInput::Unavailable(Outage::GaveUp {
            report: REPORT.to_owned(),
        })],
        "stopped" => vec![StartupInput::Unavailable(Outage::Stopped)],
        _ => Vec::new(),
    }
}

pub(crate) fn timing_for(phase: &str) -> StartupTiming {
    if phase == DEMO_PHASE {
        return StartupTiming::default();
    }
    StartupTiming::new(Duration::ZERO, Duration::ZERO)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_phase_feeds_its_inputs_and_the_demo_feeds_none() {
        const CASES: &[(&str, usize)] = &[
            ("starting", 1),
            ("migrating", 1),
            ("halted", 1),
            ("restarting", 1),
            ("unresponsive", 1),
            ("gave-up", 1),
            ("stopped", 1),
            (DEMO_PHASE, 0),
        ];
        for &(phase, count) in CASES {
            assert_eq!(phase_input(phase).len(), count, "{phase}");
        }
    }

    #[test]
    fn the_demo_keeps_the_default_timing_and_the_phases_show_at_once() {
        assert_eq!(timing_for(DEMO_PHASE), StartupTiming::default());
        assert_ne!(timing_for("halted"), StartupTiming::default());
    }
}
