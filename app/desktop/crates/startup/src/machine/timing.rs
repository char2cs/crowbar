use std::time::Duration;

/// How long the view stays blank and how long a screen, once visible, stays before the next
/// normal screen replaces it. Failures are never held.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Timing {
    reveal_delay: Duration,
    min_visible: Duration,
}

impl Timing {
    /// The blank time before anything is drawn, so a start that finishes at once shows nothing.
    pub const DEFAULT_REVEAL_DELAY: Duration = Duration::from_millis(250);
    /// The least time a revealed screen stays up, so a short phase does not flash.
    pub const DEFAULT_MIN_VISIBLE: Duration = Duration::from_millis(600);

    /// Timing with the given blank time and minimum visible time.
    pub const fn new(
        reveal_delay: Duration,
        min_visible: Duration,
    ) -> Self {
        Self {
            reveal_delay,
            min_visible,
        }
    }

    /// The blank time before the first screen is drawn.
    pub fn reveal_delay(&self) -> Duration {
        self.reveal_delay
    }

    /// The least time a revealed screen stays up.
    pub fn min_visible(&self) -> Duration {
        self.min_visible
    }
}

impl Default for Timing {
    fn default() -> Self {
        Self::new(Self::DEFAULT_REVEAL_DELAY, Self::DEFAULT_MIN_VISIBLE)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn default_reveals_late_and_holds_longer_than_it_reveals() {
        let timing = Timing::default();
        assert_eq!(timing.reveal_delay(), Timing::DEFAULT_REVEAL_DELAY);
        assert_eq!(timing.min_visible(), Timing::DEFAULT_MIN_VISIBLE);
        assert!(timing.min_visible() > timing.reveal_delay());
    }
}
