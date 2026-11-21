//! Builders the tab bar's tests share.

use gpui::{point, px};
use protocol::PaneId;

use crate::tabbar::strip_drag::StripDrag;

const CENTRES: [f32; 3] = [50.0, 150.0, 250.0];

pub(crate) fn dragged(
    from: usize,
    moves: &[f32],
) -> StripDrag {
    let press = point(px(CENTRES.get(from).copied().unwrap_or_default()), px(16.0));
    let pane = PaneId::new("a").expect("id");
    let mut drag = StripDrag::new((pane, from), press, CENTRES.map(px).to_vec());
    for shift in moves {
        drag.moved(point(press.x + px(*shift), press.y));
    }
    drag
}
