use crate::input::geometry::floor_cells;

#[derive(Debug, Clone, Copy, Default)]
pub struct ScrollAccumulator {
    remainder: f32,
}

impl ScrollAccumulator {
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

    pub fn reset(&mut self) {
        self.remainder = 0.0;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fractions_accumulate_into_whole_lines() {
        let mut accumulator = ScrollAccumulator::default();
        let moved: Vec<i32> = [0.4, 0.4, 0.4, -2.5]
            .into_iter()
            .map(|delta| accumulator.add(delta))
            .collect();
        assert_eq!(moved, [0, 0, 1, -2]);
        accumulator.reset();
        assert_eq!(accumulator.add(0.3), 0);
    }
}
