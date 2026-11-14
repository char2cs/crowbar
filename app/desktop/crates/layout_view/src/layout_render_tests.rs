use model::Axis;

use super::*;

#[test]
fn model_axes_map_to_the_matching_sash_axes() {
    assert_eq!(
        ui_axis(Axis::Horizontal),
        ui::Axis::Horizontal,
        "side by side"
    );
    assert_eq!(ui_axis(Axis::Vertical), ui::Axis::Vertical, "stacked");
}
