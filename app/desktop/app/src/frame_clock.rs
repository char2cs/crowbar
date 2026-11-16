use std::collections::VecDeque;
use std::time::{Duration, Instant};

use crate::frame_stats::FrameStats;

const WINDOW: Duration = Duration::from_secs(1);
const DROP_GAP: Duration = Duration::from_millis(34);

/// Remembers when the window painted during the last second.
#[derive(Debug, Default)]
pub(crate) struct FrameClock {
    painted: VecDeque<Instant>,
}

impl FrameClock {
    pub(crate) fn record(
        &mut self,
        at: Instant,
    ) {
        self.painted.push_back(at);
        self.forget_before(at);
    }

    pub(crate) fn stats(
        &mut self,
        now: Instant,
    ) -> FrameStats {
        self.forget_before(now);
        let gaps: Vec<Duration> = self
            .painted
            .iter()
            .zip(self.painted.iter().skip(1))
            .map(|(first, second)| second.duration_since(*first))
            .collect();
        FrameStats {
            fps: self.painted.len(),
            max_gap_ms: gaps
                .iter()
                .max()
                .map_or(0, |gap| u64::try_from(gap.as_millis()).unwrap_or(u64::MAX)),
            drops: gaps.iter().filter(|gap| **gap > DROP_GAP).count(),
        }
    }

    fn forget_before(
        &mut self,
        now: Instant,
    ) {
        while self
            .painted
            .front()
            .is_some_and(|oldest| now.duration_since(*oldest) > WINDOW)
        {
            self.painted.pop_front();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_stats_cover_the_last_second_with_its_longest_gap_and_slow_frames() {
        let start = Instant::now();
        let mut clock = FrameClock::default();
        assert_eq!(clock.stats(start), FrameStats::default());
        for offset_ms in [0, 16, 32, 100, 116] {
            clock.record(start + Duration::from_millis(offset_ms));
        }
        let now = start + Duration::from_millis(120);
        assert_eq!(
            clock.stats(now),
            FrameStats {
                fps: 5,
                max_gap_ms: 68,
                drops: 1
            }
        );
        let later = start + Duration::from_millis(1110);
        assert_eq!(clock.stats(later).fps, 1);
        assert_eq!(clock.stats(start + Duration::from_secs(5)).fps, 0);
    }
}
