//! How long to wait before reattaching to a stream that dropped.

use std::time::Duration;

const MAX_ATTEMPTS: u32 = 8;
const FIRST_DELAY: Duration = Duration::from_millis(500);
const LONGEST_DELAY: Duration = Duration::from_secs(8);

#[derive(Debug, Default)]
pub(crate) struct Reconnect {
    failures: u32,
}

impl Reconnect {
    pub(crate) fn connected(&mut self) {
        self.failures = 0;
    }

    pub(crate) fn next_delay(&mut self) -> Option<Duration> {
        if self.failures >= MAX_ATTEMPTS {
            return None;
        }
        let delay = FIRST_DELAY
            .saturating_mul(2u32.saturating_pow(self.failures))
            .min(LONGEST_DELAY);
        self.failures += 1;
        Some(delay)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [u64; 8] = [500, 1000, 2000, 4000, 8000, 8000, 8000, 8000];

    #[test]
    fn delays_double_up_to_a_cap_then_give_up() {
        let mut reconnect = Reconnect::default();
        for (attempt, millis) in CASES.iter().enumerate() {
            let delay = reconnect.next_delay();
            assert_eq!(delay, Some(Duration::from_millis(*millis)), "{attempt}");
        }
        assert_eq!(reconnect.next_delay(), None);
    }

    #[test]
    fn a_connection_starts_the_count_over() {
        let mut reconnect = Reconnect::default();
        reconnect.next_delay();
        reconnect.next_delay();
        reconnect.connected();
        assert_eq!(reconnect.next_delay(), Some(FIRST_DELAY));
    }
}
