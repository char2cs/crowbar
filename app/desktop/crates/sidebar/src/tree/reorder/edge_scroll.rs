//! Auto-scroll of the tree while a row is dragged near its top or bottom edge.

const EDGE_BAND: f32 = 36.0;
const EDGE_SPEED_MAX: f32 = 14.0;

fn edge_speed(distance: f32) -> f32 {
    if !(0.0..EDGE_BAND).contains(&distance) {
        return 0.0;
    }
    ((1.0 - distance / EDGE_BAND) * EDGE_SPEED_MAX)
        .round()
        .max(1.0)
}

/// Auto-scroll of a tree spanning `top` to `bottom` while a row is dragged near its edges.
#[derive(Debug, Clone, Copy, PartialEq)]
pub struct EdgeScroll {
    top: f32,
    bottom: f32,
}

impl EdgeScroll {
    /// The auto-scroll of a tree spanning `top` to `bottom`.
    pub fn between((top, bottom): (f32, f32)) -> Self {
        Self { top, bottom }
    }

    /// The scroll speed, in pixels per frame, for a drag at height `y`: negative near the top,
    /// positive near the bottom and zero in the middle.
    pub fn pull(
        self,
        y: f32,
    ) -> f32 {
        edge_speed(self.bottom - y) - edge_speed(y - self.top)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_pull_points_towards_the_nearer_edge() {
        const CASES: [(f32, f32); 4] = [(5.0, -12.0), (95.0, 12.0), (50.0, 0.0), (100.0, 14.0)];
        for (y, expected) in CASES {
            let got = EdgeScroll::between((0.0, 100.0)).pull(y);
            assert!((got - expected).abs() < 0.01, "y {y}: {got}");
        }
    }

    #[test]
    fn speed_ramps_from_fourteen_at_the_edge_to_one_at_the_band() {
        let cases = [
            (0.0, 14.0),
            (18.0, 7.0),
            (35.9, 1.0),
            (36.0, 0.0),
            (-1.0, 0.0),
            (100.0, 0.0),
        ];
        for (distance, expected) in cases {
            assert!(
                (edge_speed(distance) - expected).abs() < f32::EPSILON,
                "distance {distance}"
            );
        }
    }
}
