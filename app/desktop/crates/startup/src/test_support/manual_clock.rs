use std::cell::Cell;
use std::time::Duration;

use crate::view::Clock;

/// A clock the test moves by hand.
#[derive(Debug, Default)]
pub(crate) struct ManualClock {
    now: Cell<Duration>,
}

impl ManualClock {
    /// Sets the time.
    pub(crate) fn set(
        &self,
        now: Duration,
    ) {
        self.now.set(now);
    }
}

impl Clock for ManualClock {
    fn now(&self) -> Duration {
        self.now.get()
    }
}
