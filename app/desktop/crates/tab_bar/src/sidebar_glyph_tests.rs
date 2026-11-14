use super::*;

#[test]
fn the_rail_column_and_both_strokes_fit_inside_the_frame() {
    const {
        assert!(
            RAIL_COLUMN + STROKE * 2.0 <= WIDTH,
            "rails sit left of the divider inside the frame"
        );
        assert!(RAIL <= RAIL_COLUMN, "a rail is no wider than its column");
        assert!(
            2.0 * RAIL_HEIGHT + RAIL_GAP * 2.0 + 1.0 <= HEIGHT,
            "both rails fit the frame height"
        );
    }
}
