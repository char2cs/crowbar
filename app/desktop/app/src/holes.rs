use layout_view::{ChromeContext, PaneSource, pane_holes_with};
use model::{Rect, Tab};
use pane_host::VisiblePane;
use webview::Bounds;

use crate::single_precision::single_precision;

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
    use layout_view::{PaneSource, SidebarEdge};
    use model::{Command, Layout, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;

    struct NoChrome;

    impl PaneSource for NoChrome {
        fn content(
            &self,
            _pane: &PaneId,
            _window: &mut gpui::Window,
            _cx: &mut gpui::App,
        ) -> gpui::AnyElement {
            gpui::IntoElement::into_any_element(gpui::Empty)
        }

        fn label(
            &self,
            _pane: &PaneId,
        ) -> gpui::SharedString {
            "pane".into()
        }

        fn fresh_tab(
            &self,
            _pane: &PaneId,
        ) -> (TabId, String) {
            (TabId(99), String::new())
        }
    }

    fn chat_instance(id: &str) -> PaneInstance {
        PaneInstance {
            pane_id: PaneId::new(id).unwrap(),
            type_id: PaneTypeId::new("core.chat").unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn one_pane_layout() -> Layout {
        let mut layout = Layout::default();
        layout
            .apply(Command::AddTab {
                id: TabId(1),
                title: "t".into(),
            })
            .unwrap();
        layout
            .apply(Command::OpenPane {
                tab: TabId(1),
                instance: chat_instance("p1"),
                placement: None,
            })
            .unwrap();
        layout
    }

    #[test]
    fn holes_sit_inside_the_area_in_window_coordinates() {
        let layout = one_pane_layout();
        let area = Bounds {
            x: 294.0,
            y: 42.0,
            width: 800.0,
            height: 600.0,
        };
        let chrome = ChromeContext {
            sidebar: SidebarEdge::Left,
            tab_strip: true,
        };
        let panes = visible_panes(layout.active_tab(), area, (chrome, &NoChrome));
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
    }

    #[test]
    fn no_tab_means_no_panes() {
        let area = Bounds {
            x: 0.0,
            y: 0.0,
            width: 10.0,
            height: 10.0,
        };
        assert!(
            visible_panes(None, area, (ChromeContext::default(), &NoChrome)).is_empty(),
            "none"
        );
    }
}
