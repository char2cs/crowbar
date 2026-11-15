//! Auto-scroll of the tree while a row is dragged near its top or bottom edge.

use crate::metrics::{EDGE_BAND, EDGE_SPEED_MAX};

/// Pixels to scroll per animation frame for a pointer `distance` pixels from the nearer edge of
/// the viewport; zero outside the 36px band, 14 at the edge and 1 at the band's inner border.
pub fn edge_speed(distance: f32) -> f32 {
    if !(0.0..EDGE_BAND).contains(&distance) {
        return 0.0;
    }
    ((1.0 - distance / EDGE_BAND) * EDGE_SPEED_MAX)
        .round()
        .max(1.0)
}

#[cfg(test)]
mod tests {
    use super::*;

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
