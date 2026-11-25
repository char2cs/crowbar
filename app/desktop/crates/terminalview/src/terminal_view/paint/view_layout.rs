//! Where the grid and the scrollbar sit inside the view.

use gpui::{Bounds, Pixels, Point, point, px, size};
use terminal::CellMetrics;

const LEFT_INSET: f32 = 16.0;
const SCROLLBAR_WIDTH: f32 = 14.0;
const THUMB_WIDTH: f32 = 8.0;
const HALF: f32 = 2.0;

#[derive(Debug, Clone, Copy, PartialEq)]
pub(crate) struct ViewLayout {
    pub(crate) origin: Point<Pixels>,
    pub(crate) metrics: CellMetrics,
    pub(crate) track: Bounds<Pixels>,
}

impl ViewLayout {
    pub(crate) fn new(
        bounds: Bounds<Pixels>,
        cell: (f32, f32),
        inset: f32,
    ) -> Self {
        let width = (f32::from(bounds.size.width) - inset - SCROLLBAR_WIDTH).max(0.0);
        let height = f32::from(bounds.size.height);
        let (columns, lines) = CellMetrics::fit(cell.0, cell.1, width, height);
        let metrics = CellMetrics {
            cell_width: cell.0,
            line_height: cell.1,
            columns,
            lines,
        };
        let track = Bounds::new(
            point(bounds.right() - px(SCROLLBAR_WIDTH), bounds.top()),
            size(px(SCROLLBAR_WIDTH), px(whole_lines(metrics, height))),
        );
        Self {
            origin: bounds.origin + point(px(inset), px(0.0)),
            metrics,
            track,
        }
    }

    pub(crate) fn default_inset() -> f32 {
        LEFT_INSET
    }

    pub(crate) fn thumb_bounds(
        &self,
        top: f32,
        length: f32,
    ) -> Bounds<Pixels> {
        let inset = (SCROLLBAR_WIDTH - THUMB_WIDTH) / HALF;
        Bounds::new(
            self.track.origin + point(px(inset), px(top)),
            size(px(THUMB_WIDTH), px(length)),
        )
    }

    pub(crate) fn local(
        &self,
        position: Point<Pixels>,
    ) -> (f32, f32) {
        (
            f32::from(position.x - self.origin.x),
            f32::from(position.y - self.origin.y),
        )
    }
}

fn whole_lines(
    metrics: CellMetrics,
    height: f32,
) -> f32 {
    u16::try_from(metrics.lines).map_or(height, |lines| f32::from(lines) * metrics.line_height)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_grid_fits_between_the_inset_and_the_scrollbar() {
        let bounds = Bounds::new(point(px(100.0), px(50.0)), size(px(430.0), px(410.0)));
        let layout = ViewLayout::new(bounds, (8.0, 16.0), ViewLayout::default_inset());
        assert_eq!((layout.metrics.columns, layout.metrics.lines), (50, 25));
        assert_eq!(layout.origin, point(px(116.0), px(50.0)));
        assert_eq!(layout.track.size.width, px(14.0), "xterm 6's scrollbar");
        assert_eq!(layout.track.origin.x, px(516.0));
        assert_eq!(
            layout.track.size.height,
            px(400.0),
            "the track ends at the last whole line, as xterm's scrollable element"
        );
        let thumb = layout.thumb_bounds(0.0, 10.0);
        assert_eq!(
            (thumb.origin.x, thumb.size.width),
            (px(519.0), px(8.0)),
            "the app's 3px inset thumb"
        );
    }

    #[test]
    fn local_coordinates_are_relative_to_the_grid() {
        let bounds = Bounds::new(point(px(0.0), px(0.0)), size(px(300.0), px(100.0)));
        let layout = ViewLayout::new(bounds, (8.0, 16.0), 0.0);
        assert_eq!(layout.local(point(px(24.0), px(32.0))), (24.0, 32.0));
    }
}
