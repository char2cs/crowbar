use std::time::{Duration, Instant, SystemTime};

use crate::shell::ports::clock::Clock;

#[derive(Debug, Clone, Copy)]
pub(crate) struct FixedClock {
    base: Instant,
    seconds_since_epoch: u64,
}

impl FixedClock {
    pub(crate) fn at(seconds_since_epoch: u64) -> Self {
        Self {
            base: Instant::now(),
            seconds_since_epoch,
        }
    }
}

impl Clock for FixedClock {
    fn wall(&self) -> SystemTime {
        SystemTime::UNIX_EPOCH + Duration::from_secs(self.seconds_since_epoch)
    }

    fn instant(&self) -> Instant {
        self.base
    }
}
