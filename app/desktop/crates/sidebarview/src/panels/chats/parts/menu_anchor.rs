//! Where a button's menu opens: under its left edge, as develop anchors its "…" menus.

use std::cell::Cell;
use std::rc::Rc;

use gpui::{Bounds, IntoElement, Pixels, Point, Styled, canvas, point, px};

const MENU_GAP: f32 = 4.0;

#[derive(Clone, Default)]
pub(crate) struct MenuAnchor(Rc<Cell<Bounds<Pixels>>>);

impl MenuAnchor {
    pub(crate) fn probe(&self) -> impl IntoElement {
        let laid_out = Rc::clone(&self.0);
        canvas(move |bounds, _, _| laid_out.set(bounds), |_, (), _, _| {})
            .absolute()
            .top_0()
            .left_0()
            .size_full()
    }

    pub(crate) fn below(&self) -> Point<Pixels> {
        let bounds = self.0.get();
        point(bounds.left(), bounds.bottom() + px(MENU_GAP))
    }
}

#[cfg(test)]
mod tests {
    use gpui::size;

    use super::*;

    #[test]
    fn the_menu_opens_four_pixels_under_the_left_edge() {
        let anchor = MenuAnchor::default();
        anchor.0.set(Bounds::new(
            point(px(10.0), px(300.0)),
            size(px(200.0), px(30.0)),
        ));
        assert_eq!(anchor.below(), point(px(10.0), px(334.0)));
    }
}
