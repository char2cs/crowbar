//! Turns fractional wheel and trackpad deltas into whole lines.

use crate::input::geometry::floor_cells;

/// Carries the remainder of a scroll delta between events so slow trackpad motion is not lost.
#[derive(Debug, Clone, Copy, Default)]
pub struct ScrollAccumulator {
    remainder: f32,
}

impl ScrollAccumulator {
    /// Adds `lines` (positive scrolls up) and returns the whole lines to move now.
    pub fn add(
        &mut self,
        lines: f32,
    ) -> i32 {
        self.remainder += lines;
        let whole = self.remainder.trunc();
        self.remainder -= whole;
        let moved = i32::try_from(floor_cells(whole.abs())).unwrap_or(i32::MAX);
        if whole < 0.0 {
            return -moved;
        }
        moved
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fractions_accumulate_into_whole_lines() {
        let mut accumulator = ScrollAccumulator::default();
        let moved: Vec<i32> = [0.4, 0.4, 0.4, -2.5, 0.3]
            .into_iter()
            .map(|delta| accumulator.add(delta))
            .collect();
        assert_eq!(moved, [0, 0, 1, -2, 0]);
    }
}
