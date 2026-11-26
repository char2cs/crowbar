use layout::{DEFAULT_CHAT_SHARE, PaneGroup, Presentation, Rect, Tab};
use protocol::PaneId;

use crate::geometry::chrome_context::ChromeContext;
use crate::geometry::pane_frame::PaneFrame;
use crate::host::pane_hole::PaneHole;
use crate::host::pane_source::PaneSource;

fn pane_holes(
    tab: &Tab,
    content: Rect,
    chrome: ChromeContext,
) -> Vec<PaneHole> {
    tab.leaves(content)
        .into_iter()
        .flat_map(|(rect, group)| {
            let hole = PaneFrame::new(rect, content, chrome).hole;
            let shown = Presentation::of(group, hole, chrome.side, DEFAULT_CHAT_SHARE);
            holes_of(group, &shown)
        })
        .collect()
}

fn holes_of(
    group: &PaneGroup,
    shown: &Presentation,
) -> Vec<PaneHole> {
    let hole = |pane: Option<&PaneId>, rect: Option<Rect>| {
        pane.cloned()
            .zip(rect)
            .map(|(pane, rect)| PaneHole { pane, rect })
    };
    [
        hole(group.chat(), shown.chat),
        hole(group.active_tab(), shown.editor),
    ]
    .into_iter()
    .flatten()
    .collect()
}

/// The content hole of every pane of `tab` when the layout fills `content`. The shell gives
/// these rectangles to the pane host in place of the model's raw slots, so web views sit inside
/// the pane borders the view draws. A native pane gets no hole because no web view is placed for
/// it, and a pane with a chrome strip gets a hole that starts below it.
#[must_use]
pub fn pane_holes_with(
    tab: &Tab,
    content: Rect,
    chrome: ChromeContext,
    source: &dyn PaneSource,
) -> Vec<PaneHole> {
    pane_holes(tab, content, chrome)
        .into_iter()
        .filter(|hole| !source.is_native(&hole.pane))
        .map(|hole| below_chrome(hole, source))
        .collect()
}

fn below_chrome(
    hole: PaneHole,
    source: &dyn PaneSource,
) -> PaneHole {
    let strip = source
        .chrome_height(&hole.pane)
        .clamp(0.0, hole.rect.height);
    let rect = Rect {
        y: hole.rect.y + strip,
        height: hole.rect.height - strip,
        ..hole.rect
    };
    PaneHole { rect, ..hole }
}

#[cfg(test)]
mod tests {
    use layout::{Edge, Rect};

    use super::*;
    use crate::geometry::sidebar_edge::SidebarEdge;
    use crate::test_support::{FakeSource, layout_of, pane_id};

    const CONTENT: Rect = Rect {
        x: 296.0,
        y: 43.0,
        width: 904.0,
        height: 757.0,
    };
    const CHROME: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Left,
        side: layout::SidebarSide::Left,
        tab_strip: true,
    };

    #[test]
    fn every_pane_gets_a_hole_inside_its_borders() {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let tab = layout.showing("").expect("tab");
        let holes = pane_holes(tab, CONTENT, CHROME);
        let slots = tab.layout(CONTENT);
        assert_eq!(holes.len(), 2, "two panes");
        for (hole, slot) in holes.iter().zip(&slots) {
            assert_eq!(hole.pane, slot.pane, "same order as the model");
            let inside = hole.rect.x >= slot.rect.x && hole.rect.y >= slot.rect.y;
            assert!(
                inside && hole.rect.width < slot.rect.width,
                "{hole:?} in {slot:?}"
            );
        }
        let lone = layout_of("a", &[]);
        let holes = pane_holes(lone.showing("").expect("tab"), CONTENT, CHROME);
        let expected = Rect {
            x: 297.0,
            y: 45.0,
            width: 903.0,
            height: 755.0,
        };
        assert_eq!(
            holes[0].rect, expected,
            "a lone pane inside the left and top border"
        );
    }

    /// Each pane with a hole, and how far its hole starts below the plain one.
    fn shifts(
        plain: &[PaneHole],
        holes: &[PaneHole],
    ) -> Vec<(String, f32)> {
        let shift = |hole: &PaneHole| {
            let base = plain
                .iter()
                .find(|base| base.pane == hole.pane)
                .expect("plain");
            assert_eq!(
                hole.rect.y - base.rect.y,
                base.rect.height - hole.rect.height,
                "shrinks"
            );
            (hole.pane.to_string(), hole.rect.y - base.rect.y)
        };
        holes.iter().map(shift).collect()
    }

    #[test]
    fn the_source_decides_which_panes_get_a_hole_and_where_it_starts() {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let tab = layout.showing("").expect("tab");
        let plain = pane_holes(tab, CONTENT, CHROME);
        let source = FakeSource::starting_at_first_tab;
        let cases = [
            (source(), vec![("a", 0.0), ("b", 0.0)]),
            (source().native(pane_id("a")), vec![("b", 0.0)]),
            (
                source().with_chrome(pane_id("b"), 40.0),
                vec![("a", 0.0), ("b", 40.0)],
            ),
        ];
        for (source, expected) in cases {
            let holes = pane_holes_with(tab, CONTENT, CHROME, &source);
            let expected: Vec<_> = expected
                .iter()
                .map(|(pane, by)| ((*pane).to_owned(), *by))
                .collect();
            assert_eq!(shifts(&plain, &holes), expected);
        }
    }

    #[test]
    fn a_chat_and_its_active_editor_tab_each_get_a_hole_the_tab_below_its_tab_bar() {
        let mut layout = layout_of("a", &[]);
        for name in ["t1", "t2"] {
            crate::test_support::open_tab(&mut layout, name);
        }
        let tab = layout.showing("").expect("tab");
        let holes = pane_holes(tab, CONTENT, CHROME);
        let lone = pane_holes(
            layout_of("a", &[]).showing("").expect("tab"),
            CONTENT,
            CHROME,
        );
        let panes: Vec<_> = holes.iter().map(|hole| hole.pane.clone()).collect();
        assert_eq!(
            panes,
            [pane_id("a"), pane_id("t2")],
            "the chat and the active tab"
        );
        let (chat, editor) = (holes[0].rect, holes[1].rect);
        assert!(chat.width < lone[0].rect.width, "the chat shares the group");
        assert!(editor.x > chat.x + chat.width, "the editor past the sash");
        assert_eq!(
            editor.y - chat.y,
            layout::TOP_ROW_HEIGHT,
            "below the tab bar"
        );
    }

    #[test]
    fn a_closed_sidebar_docked_on_the_right_keeps_the_chat_on_the_right() {
        let mut layout = layout_of("a", &[]);
        crate::test_support::open_tab(&mut layout, "t1");
        let tab = layout.showing("").expect("tab");
        let chrome = ChromeContext {
            sidebar: SidebarEdge::Closed,
            side: layout::SidebarSide::Right,
            ..CHROME
        };
        let holes = pane_holes(tab, CONTENT, chrome);
        assert!(
            holes[0].rect.x > holes[1].rect.x,
            "the chat right of the editor: {holes:?}"
        );
    }
}
