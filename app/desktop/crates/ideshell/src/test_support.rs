//! Builders the tab bar's tests share.

use gpui::{Bounds, Pixels, point, px, size};
use protocol::PaneId;

use crate::tabbar::strip_drag::StripDrag;

const TAB: f32 = 100.0;
const LEFTS: [f32; 3] = [0.0, TAB, 2.0 * TAB];

fn tab(index: usize) -> (usize, Bounds<Pixels>) {
    let left = LEFTS.get(index).copied().unwrap_or_default();
    (
        index,
        Bounds::new(point(px(left), px(0.0)), size(px(TAB), px(32.0))),
    )
}

pub(crate) fn dragged(
    from: usize,
    moves: &[f32],
) -> StripDrag {
    let left = LEFTS.get(from).copied().unwrap_or_default();
    let press = point(px(left + 50.0), px(16.0));
    let mut drag = StripDrag::new((PaneId::new("a").expect("id"), from), press, 3);
    let steps = moves
        .iter()
        .flat_map(|shift| (0..LEFTS.len()).map(move |index| (*shift, index)));
    for (shift, index) in steps {
        drag.moved(tab(index), point(press.x + px(shift), press.y));
    }
    drag
}
