use std::time::{Instant, SystemTime};

use crate::clock::Clock;

#[derive(Debug, Clone, Copy, Default)]
pub(crate) struct SystemClock(());

impl Clock for SystemClock {
    fn wall(&self) -> SystemTime {
        SystemTime::now()
    }

    fn instant(&self) -> Instant {
        Instant::now()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_clock_reads_a_time_after_the_epoch_and_never_goes_back() {
        let clock = SystemClock::default();
        let first = clock.instant();
        assert!(clock.wall() > SystemTime::UNIX_EPOCH, "wall time");
        assert!(clock.instant() >= first, "monotonic");
    }
}
