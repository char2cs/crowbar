use std::time::{Duration, Instant};

use crate::view::clock::Clock;

/// The real clock: time since it was created.
#[derive(Debug, Clone, Copy)]
pub struct MonotonicClock {
    origin: Instant,
}

impl MonotonicClock {
    /// A clock whose origin is now.
    pub fn start() -> Self {
        Self {
            origin: Instant::now(),
        }
    }
}

impl Clock for MonotonicClock {
    fn now(&self) -> Duration {
        self.origin.elapsed()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn time_never_goes_backwards() {
        let clock = MonotonicClock::start();
        let first = clock.now();
        assert!(clock.now() >= first);
    }
}
