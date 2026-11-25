use layout::{DEFAULT_CHAT_SHARE, PaneGroup, Presentation, Rect, Tab};
use protocol::PaneId;

use crate::geometry::chrome_context::ChromeContext;
use crate::geometry::pane_frame::PaneFrame;
use crate::host::pane_hole::PaneHole;
use crate::host::pane_source::PaneSource;

/// The content hole of every pane of `tab` when the layout fills `content`. The shell gives
/// these rectangles to the pane host in place of the model's raw slots, so web views sit inside
/// the pane borders the view draws.
#[must_use]
pub fn pane_holes(
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

/// Like [`pane_holes`], but asks `source` about each pane: a native pane gets no hole because no
/// web view is placed for it, and a pane with a chrome strip gets a hole that starts below it.
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
    fn every_pane_gets_a_hole_inside_its_slot() {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let tab = layout.showing("").expect("tab");
        let holes = pane_holes(tab, CONTENT, CHROME);
        let slots = tab.layout(CONTENT);
        assert_eq!(holes.len(), 2, "two panes");
        for (hole, slot) in holes.iter().zip(&slots) {
            assert_eq!(hole.pane, slot.pane, "same order as the model");
            assert!(
                hole.rect.x >= slot.rect.x && hole.rect.y >= slot.rect.y,
                "inside top left"
            );
            assert!(
                hole.rect.width < slot.rect.width,
                "borders and margins take space"
            );
        }
    }

    #[test]
    fn a_lone_pane_hole_is_inside_the_left_and_top_border() {
        let layout = layout_of("a", &[]);
        let tab = layout.showing("").expect("tab");
        let holes = pane_holes(tab, CONTENT, CHROME);
        let expected = Rect {
            x: 297.0,
            y: 45.0,
            width: 903.0,
            height: 755.0,
        };
        assert_eq!(
            holes.first().map(|hole| (hole.pane.clone(), hole.rect)),
            Some((pane_id("a"), expected))
        );
    }

    struct Case {
        name: &'static str,
        native: &'static [&'static str],
        chrome: &'static [(&'static str, f32)],
        panes: &'static [&'static str],
        strip_on_b: f32,
    }

    const CASES: &[Case] = &[
        Case {
            name: "a source with nothing special leaves both holes whole",
            native: &[],
            chrome: &[],
            panes: &["a", "b"],
            strip_on_b: 0.0,
        },
        Case {
            name: "a native pane gets no hole",
            native: &["a"],
            chrome: &[],
            panes: &["b"],
            strip_on_b: 0.0,
        },
        Case {
            name: "a chrome strip pushes the hole down",
            native: &[],
            chrome: &[("b", 40.0)],
            panes: &["a", "b"],
            strip_on_b: 40.0,
        },
    ];

    fn source_of(case: &Case) -> FakeSource {
        let source = case
            .native
            .iter()
            .fold(FakeSource::starting_at_first_tab(), |source, name| {
                source.native(pane_id(name))
            });
        case.chrome.iter().fold(source, |source, (name, height)| {
            source.with_chrome(pane_id(name), *height)
        })
    }

    fn shift_between(
        plain: &PaneHole,
        moved: &PaneHole,
    ) -> (f32, f32) {
        (
            moved.rect.y - plain.rect.y,
            plain.rect.height - moved.rect.height,
        )
    }

    #[test]
    fn the_source_decides_which_panes_get_a_hole_and_where_it_starts() {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let tab = layout.showing("").expect("tab");
        let plain = pane_holes(tab, CONTENT, CHROME);
        for case in CASES {
            let holes = pane_holes_with(tab, CONTENT, CHROME, &source_of(case));
            let panes: Vec<_> = holes.iter().map(|hole| hole.pane.clone()).collect();
            let expected: Vec<_> = case.panes.iter().map(|name| pane_id(name)).collect();
            assert_eq!(panes, expected, "{}", case.name);
            let b = holes.iter().find(|hole| hole.pane == pane_id("b"));
            let unshifted = &plain[1];
            let shifted = b.map(|hole| shift_between(unshifted, hole));
            assert_eq!(
                shifted,
                Some((case.strip_on_b, case.strip_on_b)),
                "{}",
                case.name
            );
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
