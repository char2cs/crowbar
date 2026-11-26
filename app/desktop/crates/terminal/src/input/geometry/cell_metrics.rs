//! Pixel geometry of the character grid, and lossless cell count conversions.

use crate::input::geometry::cell_hit::CellHit;
use crate::input::geometry::cell_side::CellSide;
use crate::input::geometry::drag_edge::DragEdge;
use crate::input::geometry::view_point::ViewPoint;

const MIN_COLUMNS: usize = 2;
const MIN_LINES: usize = 1;
const HALF: f32 = 2.0;
const HALF_WORD_BITS: u32 = 16;
const HALF_WORD_RANGE: f32 = 65_536.0;
const SEARCH_STEPS: u32 = 32;

/// The size of one cell and the grid made of them.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct CellMetrics {
    /// Cell width in pixels.
    pub cell_width: f32,
    /// Line height in pixels.
    pub line_height: f32,
    /// Columns in the grid.
    pub columns: usize,
    /// Rows in the grid.
    pub lines: usize,
}

impl CellMetrics {
    /// How many columns and lines fit in `width` by `height` pixels (at least 2 by 1).
    pub fn fit(
        cell_width: f32,
        line_height: f32,
        width: f32,
        height: f32,
    ) -> (usize, usize) {
        (
            whole_cells(width, cell_width).max(MIN_COLUMNS),
            whole_cells(height, line_height).max(MIN_LINES),
        )
    }

    /// The cell at `x`, `y` pixels from the grid's top-left corner.
    pub fn hit(
        &self,
        x: f32,
        y: f32,
    ) -> CellHit {
        let column = whole_cells(x.max(0.0), self.cell_width).min(self.columns.saturating_sub(1));
        let row = whole_cells(y.max(0.0), self.line_height).min(self.lines.saturating_sub(1));
        let within = x.max(0.0) % self.cell_width;
        CellHit {
            point: ViewPoint { row, column },
            side: side_within(within, self.cell_width),
            edge: self.edge(y),
        }
    }

    fn edge(
        &self,
        y: f32,
    ) -> DragEdge {
        if y < 0.0 {
            return DragEdge::Above;
        }
        if y >= self.line_height * cells_f32(self.lines) {
            return DragEdge::Below;
        }
        DragEdge::Inside
    }
}

fn side_within(
    offset: f32,
    cell_width: f32,
) -> CellSide {
    if offset < cell_width / HALF {
        return CellSide::Left;
    }
    CellSide::Right
}

fn whole_cells(
    length: f32,
    cell: f32,
) -> usize {
    if cell <= 0.0 || length <= 0.0 {
        return 0;
    }
    floor_cells(length / cell)
}

/// A cell or line count as a float; counts of 2^32 and above saturate.
pub fn cells_f32(count: usize) -> f32 {
    let high = u16::try_from(count >> HALF_WORD_BITS).unwrap_or(u16::MAX);
    let low = u16::try_from(count & usize::from(u16::MAX)).unwrap_or(u16::MAX);
    f32::from(high) * HALF_WORD_RANGE + f32::from(low)
}

/// The largest whole count not above `value`; negative and NaN values give 0.
pub fn floor_cells(value: f32) -> usize {
    let upper = usize::try_from(u32::MAX).unwrap_or(usize::MAX);
    let (floor, _) = (0..SEARCH_STEPS).fold((0_usize, upper), |range, _| narrow(range, value));
    floor
}

fn narrow(
    (low, high): (usize, usize),
    value: f32,
) -> (usize, usize) {
    let middle = low + (high - low).div_ceil(2);
    if cells_f32(middle) <= value {
        return (middle, high);
    }
    (low, middle - 1)
}

#[cfg(test)]
mod tests {
    use super::*;

    const COUNT_CASES: &[(usize, f32)] = &[
        (0, 0.0),
        (80, 80.0),
        (65_536, 65_536.0),
        (200_000, 200_000.0),
    ];
    const FLOOR_CASES: &[(f32, usize)] = &[
        (-3.0, 0),
        (f32::NAN, 0),
        (0.0, 0),
        (7.9, 7),
        (80.0, 80),
        (150_000.5, 150_000),
    ];

    #[test]
    fn counts_and_floors_convert() {
        for (count, expected) in COUNT_CASES {
            assert_eq!(cells_f32(*count), *expected);
        }
        for (value, expected) in FLOOR_CASES {
            assert_eq!(floor_cells(*value), *expected);
        }
    }

    #[test]
    fn pixels_fit_and_hit_whole_cells_clamped_to_the_grid() {
        assert_eq!(CellMetrics::fit(8.0, 16.0, 100.0, 70.0), (12, 4));
        assert_eq!(CellMetrics::fit(8.0, 16.0, 1.0, 1.0), (2, 1));
        let metrics = CellMetrics {
            cell_width: 8.0,
            line_height: 16.0,
            columns: 10,
            lines: 5,
        };
        let at = |row, column| ViewPoint { row, column };
        let cases = [
            ((13.0, 20.0), (at(1, 1), CellSide::Right, DragEdge::Inside)),
            ((8.5, 0.0), (at(0, 1), CellSide::Left, DragEdge::Inside)),
            ((500.0, 500.0), (at(4, 9), CellSide::Right, DragEdge::Below)),
            ((-5.0, -5.0), (at(0, 0), CellSide::Left, DragEdge::Above)),
        ];
        for ((x, y), expected) in cases {
            let hit = metrics.hit(x, y);
            assert_eq!((hit.point, hit.side, hit.edge), expected, "{x},{y}");
        }
    }
}
