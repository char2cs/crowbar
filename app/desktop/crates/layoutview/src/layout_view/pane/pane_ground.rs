//! The fill behind a group's tab bar and editor. GPUI clips a child to a rectangle, so the fill
//! rounds the pane's corners itself and what is drawn over it paints none.

use gpui::{Div, Hsla, Styled as _, div, px};
use layout::Rect;

use crate::geometry::corners::Corners;
use crate::geometry::pane_frame::PaneFrame;

pub(crate) fn grounds(
    parts: [Option<Rect>; 2],
    frame: &PaneFrame,
) -> Vec<(Rect, Corners)> {
    parts
        .into_iter()
        .flatten()
        .map(|part| (part, frame.inner_corners(within(part, frame.hole))))
        .collect()
}

pub(crate) fn ground(
    (part, corners): (Rect, Corners),
    fill: Hsla,
) -> Div {
    div()
        .absolute()
        .left(px(part.x))
        .top(px(part.y))
        .w(px(part.width))
        .h(px(part.height))
        .rounded_tl(px(corners.top_left))
        .rounded_tr(px(corners.top_right))
        .rounded_bl(px(corners.bottom_left))
        .rounded_br(px(corners.bottom_right))
        .bg(fill)
}

fn within(
    part: Rect,
    hole: Rect,
) -> Rect {
    Rect {
        x: hole.x + part.x,
        y: hole.y + part.y,
        ..part
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::geometry::chrome_context::ChromeContext;

    #[test]
    fn a_tab_bar_rounds_the_pane_s_top_corners_and_the_editor_under_it_the_bottom_ones() {
        let slot = Rect {
            x: 100.0,
            y: 50.0,
            width: 200.0,
            height: 200.0,
        };
        let content = Rect {
            x: 0.0,
            y: 0.0,
            width: 800.0,
            height: 600.0,
        };
        let frame = PaneFrame::new(slot, content, ChromeContext::default());
        let bar = Rect {
            x: 0.0,
            y: 0.0,
            width: frame.hole.width,
            height: 42.0,
        };
        let editor = Rect {
            y: 42.0,
            height: frame.hole.height - 42.0,
            ..bar
        };
        let drawn = grounds([Some(bar), Some(editor)], &frame);
        let top = Corners {
            top_left: 9.0,
            top_right: 9.0,
            ..Corners::default()
        };
        let bottom = Corners {
            bottom_left: 9.0,
            bottom_right: 9.0,
            ..Corners::default()
        };
        assert_eq!(drawn, [(bar, top), (editor, bottom)]);
    }
}
