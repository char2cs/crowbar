use gpui::{Bounds, Modifiers, Pixels, VisualTestContext, point, px};

const STEP: f32 = 12.0;

/// Clicks a grid of points over `area`, so every button inside it is hit at least once.
pub(crate) fn click_across(
    cx: &mut VisualTestContext,
    area: Bounds<Pixels>,
) {
    let (left, top) = (f32::from(area.origin.x), f32::from(area.origin.y));
    let (width, height) = (f32::from(area.size.width), f32::from(area.size.height));
    let (columns, rows) = (steps(width), steps(height));
    let grid = (0..rows).flat_map(|row| (0..columns).map(move |column| (column, row)));
    for (column, row) in grid {
        let at = point(
            px(left + STEP * f32::from(column)),
            px(top + STEP * f32::from(row)),
        );
        cx.simulate_click(at, Modifiers::none());
    }
}

#[expect(
    clippy::cast_possible_truncation,
    clippy::cast_sign_loss,
    reason = "extents are small positive pixel counts"
)]
fn steps(extent: f32) -> u16 {
    (extent / STEP).ceil() as u16
}
