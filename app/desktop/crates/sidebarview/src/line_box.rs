//! Where the drop line sits, in window coordinates.

use gpui::{Bounds, Pixels};

use sidebar::DropMode;

const INSET: f32 = 8.0;
const TRIM: f32 = 10.0;
const LIFT: f32 = 1.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct LineBox {
    pub(crate) left: f32,
    pub(crate) top: f32,
    pub(crate) width: f32,
}

impl LineBox {
    pub(crate) fn of(
        bounds: Bounds<Pixels>,
        mode: DropMode,
    ) -> Option<Self> {
        let edge = match mode {
            DropMode::Before => bounds.top(),
            DropMode::After => bounds.bottom(),
            DropMode::Into => return None,
        };
        Some(Self {
            left: f32::from(bounds.left()) + INSET,
            top: f32::from(edge) - LIFT,
            width: (f32::from(bounds.size.width) - TRIM).max(0.0),
        })
    }
}

#[cfg(test)]
mod tests {
    use gpui::{point, px, size};

    use super::*;

    type Case = (DropMode, Option<(f32, f32)>);

    const CASES: [Case; 3] = [
        (DropMode::Before, Some((28.0, 99.0))),
        (DropMode::After, Some((28.0, 135.0))),
        (DropMode::Into, None),
    ];

    #[test]
    fn the_line_hugs_the_row_edge_inset_from_its_sides() {
        let bounds = Bounds::new(point(px(20.0), px(100.0)), size(px(200.0), px(36.0)));
        for (mode, want) in CASES {
            let got = LineBox::of(bounds, mode).map(|line| (line.left, line.top));
            assert_eq!(got, want, "{mode:?}");
        }
        let width = LineBox::of(bounds, DropMode::Before).map(|line| line.width);
        assert_eq!(width, Some(190.0), "width");
    }
}
