use std::time::{Duration, SystemTime};

use tokio::time::Instant;

use crate::failure_tracker::FailureTracker;

pub(crate) struct WatchState {
    pub(crate) tracker: FailureTracker,
    pub(crate) silent_since: Option<(Instant, Option<Duration>)>,
    pub(crate) previous_cycle: SystemTime,
}

impl WatchState {
    pub(crate) fn new(threshold: u32) -> Self {
        Self {
            tracker: FailureTracker::new(threshold),
            silent_since: None,
            previous_cycle: SystemTime::now(),
        }
    }

    pub(crate) fn forget(&mut self) {
        self.tracker.reset();
        self.silent_since = None;
    }
}

#[cfg(test)]
#[path = "watch_state_tests.rs"]
mod tests;
