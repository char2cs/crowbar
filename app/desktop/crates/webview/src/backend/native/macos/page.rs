use std::cell::Cell;

use platform::{ClipShape, PageClip};
use wry::WebView;

use crate::view::{Bounds, Corners};

const SIDES: usize = 4;

pub(crate) struct Page {
    pub(crate) view: WebView,
    pub(crate) clip: PageClip,
    corners: Cell<Corners>,
    hole: Cell<Option<Bounds>>,
}

impl Page {
    pub(crate) fn new(
        view: WebView,
        clip: PageClip,
    ) -> Self {
        Self {
            view,
            clip,
            corners: Cell::default(),
            hole: Cell::default(),
        }
    }

    pub(crate) fn set_hole(
        &self,
        hole: Option<Bounds>,
    ) {
        self.hole.set(hole);
        self.shape();
    }

    pub(crate) fn set_corners(
        &self,
        corners: Corners,
    ) {
        self.corners.set(corners);
        self.shape();
    }

    fn shape(&self) {
        self.clip
            .set_shape(clip_shape(self.corners.get(), self.hole.get()));
    }
}

fn clip_shape(
    corners: Corners,
    hole: Option<Bounds>,
) -> ClipShape {
    let Corners {
        top_left,
        top_right,
        bottom_right,
        bottom_left,
    } = corners;
    ClipShape {
        corners: [top_left, top_right, bottom_right, bottom_left],
        hole: hole.map(frame_of),
    }
}

pub(crate) fn frame_of(bounds: Bounds) -> [f64; SIDES] {
    [bounds.x, bounds.y, bounds.width, bounds.height]
}

pub(crate) fn filling(bounds: Bounds) -> Bounds {
    Bounds {
        x: 0.0,
        y: 0.0,
        ..bounds
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_page_fills_its_clip_which_rounds_its_corners_clockwise() {
        let bounds = Bounds {
            x: 3.0,
            y: 4.0,
            width: 5.0,
            height: 6.0,
        };
        let corners = Corners {
            bottom_left: 9.0,
            ..Corners::default()
        };
        let shape = clip_shape(corners, Some(bounds));
        assert_eq!(frame_of(filling(bounds)), [0.0, 0.0, 5.0, 6.0]);
        assert_eq!(shape.corners, [0.0, 0.0, 0.0, 9.0]);
        assert_eq!(shape.hole, Some([3.0, 4.0, 5.0, 6.0]));
    }
}
