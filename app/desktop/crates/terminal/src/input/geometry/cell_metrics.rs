use crate::input::geometry::cell_hit::CellHit;
use crate::input::geometry::cell_side::CellSide;
use crate::input::geometry::cell_units::{cells_f32, floor_cells};
use crate::input::geometry::drag_edge::DragEdge;
use crate::input::geometry::view_point::ViewPoint;

const MIN_COLUMNS: usize = 2;
const MIN_LINES: usize = 1;
const HALF: f32 = 2.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub struct CellMetrics {
    pub cell_width: f32,
    pub line_height: f32,
    pub columns: usize,
    pub lines: usize,
}

impl CellMetrics {
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

#[cfg(test)]
mod tests {
    use super::*;

    fn metrics() -> CellMetrics {
        CellMetrics {
            cell_width: 8.0,
            line_height: 16.0,
            columns: 10,
            lines: 5,
        }
    }

    #[test]
    fn fit_counts_whole_cells_with_a_floor() {
        assert_eq!(CellMetrics::fit(8.0, 16.0, 100.0, 70.0), (12, 4));
        assert_eq!(CellMetrics::fit(8.0, 16.0, 1.0, 1.0), (2, 1));
    }

    #[test]
    fn hit_resolves_cell_side_and_clamps() {
        let hit = metrics().hit(13.0, 20.0);
        assert_eq!(
            (hit.point, hit.side, hit.edge),
            (
                ViewPoint { row: 1, column: 1 },
                CellSide::Right,
                DragEdge::Inside
            )
        );
        let left = metrics().hit(8.5, 0.0);
        assert_eq!(left.side, CellSide::Left);
        let far = metrics().hit(500.0, 500.0);
        assert_eq!(
            (far.point, far.edge),
            (ViewPoint { row: 4, column: 9 }, DragEdge::Below)
        );
        assert_eq!(metrics().hit(-5.0, -5.0).edge, DragEdge::Above);
    }
}
