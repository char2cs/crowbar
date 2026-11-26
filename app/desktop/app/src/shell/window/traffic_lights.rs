//! Where the macOS window buttons sit: centred in the top chrome row, as in the Tauri shell.

use gpui::{Pixels, Point, point, px};

const TRAFFIC_LIGHT_X: f32 = 12.0;
const CHROME_ROW_HEIGHT: f32 = 42.0;
// PLATFORM: macos draws its standard window buttons fourteen points tall.
const TRAFFIC_LIGHT_HEIGHT: f32 = 14.0;

pub(crate) fn position() -> Point<Pixels> {
    point(px(TRAFFIC_LIGHT_X), px(top(CHROME_ROW_HEIGHT)))
}

// PLATFORM: macos window buttons get their top edge at this offset under GPUI.
fn top(row_height: f32) -> f32 {
    (row_height - TRAFFIC_LIGHT_HEIGHT) / 2.0
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_traffic_lights_are_centred_in_the_chrome_row() {
        let cases = [(CHROME_ROW_HEIGHT, 14.0), (TRAFFIC_LIGHT_HEIGHT, 0.0)];
        for (row, expected) in cases {
            assert_eq!(top(row), expected, "row {row}");
            assert_eq!(
                top(row) + TRAFFIC_LIGHT_HEIGHT / 2.0,
                row / 2.0,
                "centred in {row}"
            );
        }
    }
}
