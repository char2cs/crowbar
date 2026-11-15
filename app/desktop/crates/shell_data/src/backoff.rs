//! How long a stream waits before it dials again.

use std::time::Duration;

const FIRST: Duration = Duration::from_secs(1);
const LONGEST: Duration = Duration::from_secs(30);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) struct Backoff {
    next: Duration,
}

impl Default for Backoff {
    fn default() -> Self {
        Self { next: FIRST }
    }
}

impl Backoff {
    pub(crate) fn wait(&mut self) -> Duration {
        let wait = self.next;
        self.next = (self.next * 2).min(LONGEST);
        wait
    }

    pub(crate) fn reset(&mut self) {
        self.next = FIRST;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn waits_double_up_to_the_ceiling_and_reset_starts_over() {
        let mut backoff = Backoff::default();
        let waits: Vec<u64> = (0..7).map(|_| backoff.wait().as_secs()).collect();
        assert_eq!(waits, [1, 2, 4, 8, 16, 30, 30]);
        backoff.reset();
        assert_eq!(backoff.wait(), FIRST);
    }
}
