use gtk::prelude::WidgetExt as _;
use wry::WebViewExtUnix;
use wry::dpi::{LogicalPosition, LogicalSize, PhysicalPosition, PhysicalSize};

use platform::{Card, PageCut};

use crate::backend::native::linux::gtk_state::GtkState;
use crate::backend::native::linux::gtk_state::live_view::LiveView;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Id;
use crate::view::Visibility;

const CUT_RADIUS: f64 = 8.0;

impl GtkState {
    pub(crate) fn set_cutout(
        &self,
        id: Id,
        hole: Option<Bounds>,
    ) -> Result<()> {
        let live = self.live(id)?;
        let scale = f64::from(live.view.webview().scale_factor());
        let card = hole.map(|hole| cut_card(hole, scale));
        let was = cut_of(live.shown.get(), live.hole.replace(card));
        self.recut(live, was)
    }

    pub(super) fn recut(
        &self,
        live: &LiveView,
        was: Option<PageCut>,
    ) -> Result<()> {
        let now = cut_of(live.shown.get(), live.hole.get());
        if now == was {
            return Ok(());
        }
        Ok(self
            .page_shape()
            .cut((live.page_window, live.parent), now)?)
    }
}

pub(super) fn cut_of(
    shown: Visibility,
    hole: Option<Card>,
) -> Option<PageCut> {
    if shown == Visibility::Covered {
        return Some(PageCut::Everything);
    }
    hole.map(PageCut::Hole)
}

fn cut_card(
    hole: Bounds,
    scale: f64,
) -> Card {
    let origin: PhysicalPosition<i16> =
        LogicalPosition::new(hole.x, hole.y - CUT_RADIUS).to_physical(scale);
    let size: PhysicalSize<u16> =
        LogicalSize::new(hole.width, hole.height + CUT_RADIUS).to_physical(scale);
    let radius: PhysicalSize<u16> = LogicalSize::new(CUT_RADIUS, 0.0).to_physical(scale);
    Card {
        x: origin.x,
        y: origin.y,
        width: size.width,
        height: size.height,
        radius: radius.width,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_covered_view_is_cut_away_whole_and_otherwise_keeps_its_hole() {
        let hole = cut_card(
            Bounds {
                x: 1.0,
                y: 0.0,
                width: 2.0,
                height: 3.0,
            },
            1.0,
        );
        let cases = [
            (Visibility::Covered, Some(hole), Some(PageCut::Everything)),
            (Visibility::Covered, None, Some(PageCut::Everything)),
            (Visibility::Shown, Some(hole), Some(PageCut::Hole(hole))),
            (Visibility::Hidden, Some(hole), Some(PageCut::Hole(hole))),
            (Visibility::Shown, None, None),
        ];
        for (shown, hole, cut) in cases {
            assert_eq!(cut_of(shown, hole), cut, "{shown:?} {hole:?}");
        }
    }

    #[test]
    fn a_hole_is_cut_in_device_pixels_its_top_corners_lifted_above_the_page() {
        let hole = Bounds {
            x: 100.0,
            y: 0.0,
            width: 69.0,
            height: 20.0,
        };
        let card = cut_card(hole, 2.0);
        let expected = Card {
            x: 200,
            y: -16,
            width: 138,
            height: 56,
            radius: 16,
        };
        assert_eq!(
            card, expected,
            "doubled, only the bottom corners round on the page"
        );
    }
}
