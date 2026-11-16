use gpui::{Bounds, Modifiers, Pixels, VisualTestContext, point, px};

const STEP: f32 = 12.0;

/// Clicks a grid of points over `area`, so every button inside it is hit at least once.
pub(crate) fn click_across(
    cx: &mut VisualTestContext,
    area: Bounds<Pixels>,
) {
    let (left, top) = (f32::from(area.origin.x), f32::from(area.origin.y));
    let (right, bottom) = (
        left + f32::from(area.size.width),
        top + f32::from(area.size.height),
    );
    let along = |from: f32, to: f32| {
        std::iter::successors(Some(from), |at| Some(at + STEP)).take_while(move |at| *at < to)
    };
    let grid = along(top, bottom).flat_map(|y| along(left, right).map(move |x| (x, y)));
    for (x, y) in grid {
        cx.simulate_click(point(px(x), px(y)), Modifiers::none());
    }
}
