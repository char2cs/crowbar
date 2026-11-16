use std::time::SystemTime;

use tokio::time::Instant;

use crate::clock::Clock;

pub(crate) fn system_clock() -> Clock {
    Clock {
        instant: Instant::now,
        wall: SystemTime::now,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_system_clock_reads_a_wall_time_after_the_epoch_and_a_steady_instant() {
        let clock = system_clock();
        let first = (clock.instant)();
        assert!((clock.wall)() > SystemTime::UNIX_EPOCH);
        assert!((clock.instant)() >= first);
    }
}
