use layout::{Rect, Tab};
use layoutview::{ChromeContext, PaneSource, pane_holes_with};
use panehost::{Covered, Overlays, VisiblePane};
use webview::Bounds;

use crate::panes::content::single_precision::single_precision;
use crate::shell::window::pixels;

pub(crate) fn visible_panes(
    tab: Option<&Tab>,
    area: Bounds,
    (chrome, source): (ChromeContext, &dyn PaneSource),
) -> Vec<VisiblePane> {
    let Some(tab) = tab else {
        return Vec::new();
    };
    let local = Rect {
        x: 0.0,
        y: 0.0,
        width: single_precision(area.width),
        height: single_precision(area.height),
    };
    pane_holes_with(tab, local, chrome, source)
        .into_iter()
        .map(|hole| VisiblePane {
            bounds: placed(area, hole.rect),
            pane: hole.pane,
        })
        .collect()
}

pub(crate) fn overlays(
    cover: &ui::Cover,
    handle: Option<Rect>,
    area: Bounds,
) -> Overlays {
    Overlays {
        covered: covered(cover),
        cutout: handle.map(|rect| placed(area, rect)),
    }
}

fn covered(cover: &ui::Cover) -> Covered {
    if cover.whole() {
        return Covered::Everything;
    }
    let cards = cover.cards();
    if cards.is_empty() {
        return Covered::Nothing;
    }
    Covered::Areas(cards.into_iter().map(pixels::to_bounds).collect())
}

fn placed(
    area: Bounds,
    rect: Rect,
) -> Bounds {
    Bounds {
        x: area.x + f64::from(rect.x),
        y: area.y + f64::from(rect.y),
        width: f64::from(rect.width),
        height: f64::from(rect.height),
    }
}

#[cfg(test)]
mod tests {
    use ideshell::TabLooks;
    use layout::TabId;
    use layoutview::{PaneChrome, PaneContent, PaneDetach, SidebarEdge};
    use protocol::PaneId;

    use super::*;
    use crate::test_support::{one_tab, pane};

    struct NoChrome;

    impl PaneContent for NoChrome {
        fn content(
            &self,
            _pane: &PaneId,
            _window: &mut gpui::Window,
            _cx: &mut gpui::App,
        ) -> gpui::AnyElement {
            gpui::IntoElement::into_any_element(gpui::Empty)
        }
    }

    impl PaneChrome for NoChrome {}

    impl layoutview::BranchReview for NoChrome {}

    impl TabLooks for NoChrome {}

    impl PaneDetach for NoChrome {
        fn label(
            &self,
            _pane: &PaneId,
        ) -> gpui::SharedString {
            "pane".into()
        }

        fn preview(
            &self,
            _pane: &PaneId,
            _cx: &gpui::App,
        ) -> Option<gpui::AnyElement> {
            None
        }

        fn fresh_tab(
            &self,
            _pane: &PaneId,
        ) -> (TabId, String) {
            (TabId(99), String::new())
        }
    }

    #[test]
    fn holes_sit_inside_the_area_in_window_coordinates() {
        let layout = one_tab([pane("p1", "core.chat")]);
        let area = Bounds {
            x: 294.0,
            y: 42.0,
            width: 800.0,
            height: 600.0,
        };
        let chrome = ChromeContext {
            sidebar: SidebarEdge::Left,
            side: layout::SidebarSide::Left,
            tab_strip: true,
        };
        let panes = visible_panes(layout.showing(""), area, (chrome, &NoChrome));
        assert_eq!(panes.len(), 1, "one pane");
        let bounds = panes[0].bounds;
        assert!(
            bounds.x >= area.x && bounds.y >= area.y,
            "starts inside the area: {bounds:?}"
        );
        assert!(
            bounds.x + bounds.width <= area.x + area.width
                && bounds.y + bounds.height <= area.y + area.height,
            "ends inside the area: {bounds:?}"
        );
        let none = visible_panes(None, area, (ChromeContext::default(), &NoChrome));
        assert!(none.is_empty(), "no tab means no panes");
    }
}
