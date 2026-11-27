use gtk::prelude::WidgetExt as _;
use wry::WebViewExtUnix;
use wry::dpi::{LogicalPosition, LogicalSize, PhysicalPosition, PhysicalSize};

use platform::{Card, PageCut, Rounded};

use crate::backend::native::linux::gtk_state::GtkState;
use crate::backend::native::linux::gtk_state::live_view::LiveView;
use crate::error::Result;
use crate::view::Bounds;
use crate::view::Corners;
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
        self.reshape(live, || live.hole.set(card))
    }

    pub(crate) fn set_corners(
        &self,
        id: Id,
        corners: Corners,
    ) -> Result<()> {
        let live = self.live(id)?;
        self.reshape(live, || live.corners.set(corners))
    }

    fn reshape(
        &self,
        live: &LiveView,
        change: impl FnOnce(),
    ) -> Result<()> {
        let was = live.cut();
        change();
        self.recut(live, was)
    }

    pub(super) fn recut(
        &self,
        live: &LiveView,
        was: Option<PageCut>,
    ) -> Result<()> {
        let now = live.cut();
        if now == was {
            return Ok(());
        }
        Ok(self
            .page_shape()
            .cut((live.page_window, live.parent), now)?)
    }
}

impl LiveView {
    pub(super) fn cut(&self) -> Option<PageCut> {
        let scale = f64::from(self.view.webview().scale_factor());
        let size = self.place.size(scale);
        let round = rounded(self.corners.get(), size, scale);
        cut_of(self.shown.get(), self.hole.get(), round)
    }
}

fn cut_of(
    shown: Visibility,
    hole: Option<Card>,
    round: Option<Rounded>,
) -> Option<PageCut> {
    if shown == Visibility::Covered {
        return Some(PageCut::Everything);
    }
    let trimmed = hole.is_some() || round.is_some();
    trimmed.then_some(PageCut::Trimmed { hole, round })
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

fn rounded(
    corners: Corners,
    (width, height): (f64, f64),
    scale: f64,
) -> Option<Rounded> {
    if corners == Corners::default() {
        return None;
    }
    let size: PhysicalSize<u16> = LogicalSize::new(width, height).to_physical(scale);
    let radii = [
        corners.top_left,
        corners.top_right,
        corners.bottom_right,
        corners.bottom_left,
    ];
    let device = |radius| {
        LogicalSize::new(radius, 0.0)
            .to_physical::<u16>(scale)
            .width
    };
    Some(Rounded {
        width: size.width,
        height: size.height,
        radii: radii.map(device),
    })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn hole_and_round() -> (Card, Option<Rounded>) {
        let hole = Bounds {
            x: 1.0,
            y: 0.0,
            width: 2.0,
            height: 3.0,
        };
        let corners = Corners {
            bottom_left: 9.0,
            ..Corners::default()
        };
        (cut_card(hole, 1.0), rounded(corners, (10.0, 10.0), 1.0))
    }

    #[test]
    fn a_covered_view_is_cut_away_whole_and_otherwise_keeps_its_hole_and_corners() {
        let (hole, round) = hole_and_round();
        let trimmed = |hole, round| Some(PageCut::Trimmed { hole, round });
        let cases = [
            (
                Visibility::Covered,
                Some(hole),
                round,
                Some(PageCut::Everything),
            ),
            (Visibility::Covered, None, None, Some(PageCut::Everything)),
            (
                Visibility::Shown,
                Some(hole),
                None,
                trimmed(Some(hole), None),
            ),
            (
                Visibility::Hidden,
                Some(hole),
                round,
                trimmed(Some(hole), round),
            ),
            (Visibility::Shown, None, round, trimmed(None, round)),
            (Visibility::Shown, None, None, None),
        ];
        for (shown, hole, round, cut) in cases {
            assert_eq!(
                cut_of(shown, hole, round),
                cut,
                "{shown:?} {hole:?} {round:?}"
            );
        }
    }

    #[test]
    fn a_page_s_corners_are_rounded_in_device_pixels_unless_all_are_square() {
        let corners = Corners {
            bottom_right: 9.0,
            bottom_left: 9.0,
            ..Corners::default()
        };
        let expected = Rounded {
            width: 200,
            height: 100,
            radii: [0, 0, 18, 18],
        };
        assert_eq!(rounded(corners, (100.0, 50.0), 2.0), Some(expected));
        assert_eq!(rounded(Corners::default(), (100.0, 50.0), 2.0), None);
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
