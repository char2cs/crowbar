//! Paints the scrollbar thumb.

use gpui::{Hsla, PaintQuad, Window, fill, px};
use terminal::ScrollbarThumb;

use crate::terminal_view::paint::view_layout::ViewLayout;

const THUMB_RADIUS: f32 = 2.5;

pub(crate) fn paint_scrollbar(
    thumb: ScrollbarThumb,
    layout: &ViewLayout,
    color: Hsla,
    window: &mut Window,
) {
    window.paint_quad(thumb_quad(thumb, layout, color));
}

fn thumb_quad(
    thumb: ScrollbarThumb,
    layout: &ViewLayout,
    color: Hsla,
) -> PaintQuad {
    let mut quad = fill(layout.thumb_bounds(thumb.top, thumb.length), color);
    quad.corner_radii = px(THUMB_RADIUS).into();
    quad
}

#[cfg(test)]
mod tests {
    use gpui::{Bounds, hsla, point, size};

    use super::*;

    #[test]
    fn the_thumb_is_a_rounded_quad_inside_the_track() {
        let bounds = Bounds::new(point(px(0.0), px(0.0)), size(px(300.0), px(100.0)));
        let layout = ViewLayout::new(bounds, (8.0, 16.0), 0.0);
        let color = hsla(0.5, 0.5, 0.5, 1.0);
        let quad = thumb_quad(
            ScrollbarThumb {
                top: 10.0,
                length: 20.0,
            },
            &layout,
            color,
        );
        assert_eq!(quad.bounds, layout.thumb_bounds(10.0, 20.0), "thumb bounds");
        assert_eq!(
            quad.corner_radii,
            px(THUMB_RADIUS).into(),
            "rounded corners"
        );
    }
}
