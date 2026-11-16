use std::time::{Duration, SystemTime};

use tokio::time::Instant;

use crate::failure_tracker::FailureTracker;

pub(crate) struct WatchState {
    pub(crate) tracker: FailureTracker,
    pub(crate) silent_since: Option<(Instant, Option<Duration>)>,
    pub(crate) previous_cycle: SystemTime,
}

impl WatchState {
    pub(crate) fn new(
        threshold: u32,
        started: SystemTime,
    ) -> Self {
        Self {
            tracker: FailureTracker::new(threshold),
            silent_since: None,
            previous_cycle: started,
        }
    }

    pub(crate) fn forget(&mut self) {
        self.tracker.reset();
        self.silent_since = None;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::probe::Probe;

    #[test]
    fn forgetting_clears_the_streak_and_the_silent_stretch() {
        let mut state = WatchState::new(2, SystemTime::UNIX_EPOCH);
        state.tracker.observe(Probe::Unserved);
        state.silent_since = Some((Instant::now(), None));
        state.forget();
        assert!(state.silent_since.is_none());
        assert!(
            !state.tracker.observe(Probe::Unserved),
            "the streak restarted from zero"
        );
    }
}
