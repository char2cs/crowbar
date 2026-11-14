use std::time::{Duration, Instant};

use crate::startup_clock::StartupClock;

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

impl StartupClock for MonotonicClock {
    fn now(&self) -> Duration {
        self.origin.elapsed()
    }
}

#[cfg(test)]
#[path = "monotonic_clock_tests.rs"]
mod tests;
