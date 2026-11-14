use std::time::Duration;

/// The time source of the view: a monotonic duration since any fixed origin.
pub trait StartupClock {
    /// The time now.
    fn now(&self) -> Duration;
}

#[cfg(test)]
#[path = "startup_clock_tests.rs"]
mod tests;
