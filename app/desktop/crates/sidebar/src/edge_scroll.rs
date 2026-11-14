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
#[path = "edge_scroll_tests.rs"]
mod tests;
