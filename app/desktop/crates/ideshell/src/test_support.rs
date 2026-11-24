//! Builders the tab bar's tests share.

use gpui::{Bounds, point, px, size};
use protocol::PaneId;

use crate::tabbar::strip_drag::StripDrag;

const CENTRES: [f32; 3] = [50.0, 150.0, 250.0];
const WIDTH: f32 = 100.0;
const HEIGHT: f32 = 32.0;

pub(crate) fn dragged(
    from: usize,
    moves: &[f32],
) -> StripDrag {
    let press = point(px(CENTRES.get(from).copied().unwrap_or_default()), px(16.0));
    let pane = PaneId::new("a").expect("id");
    let slot = |centre: f32| {
        let origin = point(px(centre - WIDTH / 2.0), px(0.0));
        Bounds::new(origin, size(px(WIDTH), px(HEIGHT)))
    };
    let mut drag = StripDrag::new((pane, from), press, CENTRES.map(slot).to_vec());
    for shift in moves {
        drag.moved(point(press.x + px(*shift), press.y));
    }
    drag
}
