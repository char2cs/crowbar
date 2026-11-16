use model::{GroupSlot, Rect, Tab};

use crate::chrome_context::ChromeContext;
use crate::pane_frame::PaneFrame;
use crate::pane_hole::PaneHole;
use crate::pane_source::PaneSource;
use crate::pane_strip::STRIP_HEIGHT;

/// The content hole of every pane of `tab` when the layout fills `content`. The shell gives
/// these rectangles to the pane host in place of the model's raw slots, so web views sit inside
/// the pane borders the view draws.
#[must_use]
pub fn pane_holes(
    tab: &Tab,
    content: Rect,
    chrome: ChromeContext,
) -> Vec<PaneHole> {
    tab.groups(content)
        .into_iter()
        .map(|group| PaneHole {
            rect: below_strip(PaneFrame::new(group.rect, content, chrome).hole, &group),
            pane: group.active,
        })
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

fn below_strip(
    hole: Rect,
    group: &GroupSlot,
) -> Rect {
    if group.panes.len() < 2 {
        return hole;
    }
    let strip = STRIP_HEIGHT.min(hole.height);
    Rect {
        y: hole.y + strip,
        height: hole.height - strip,
        ..hole
    }
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
    use model::{Edge, Rect};

    use super::*;
    use crate::sidebar_edge::SidebarEdge;
    use crate::test_support::{FakeSource, layout_of, pane_id};

    const CONTENT: Rect = Rect {
        x: 296.0,
        y: 43.0,
        width: 904.0,
        height: 757.0,
    };
    const CHROME: ChromeContext = ChromeContext {
        sidebar: SidebarEdge::Left,
        tab_strip: true,
    };

    #[test]
    fn every_pane_gets_a_hole_inside_its_slot() {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let tab = layout.active_tab().expect("tab");
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
        let tab = layout.active_tab().expect("tab");
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
        let tab = layout.active_tab().expect("tab");
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
    fn a_group_with_a_strip_starts_its_hole_below_the_strip_and_hides_the_others() {
        let mut layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let stacked = crate::test_support::chat("c");
        layout
            .apply(model::Command::OpenPaneInGroup {
                instance: stacked,
                onto: pane_id("b"),
                index: None,
            })
            .expect("stack");
        let tab = layout.active_tab().expect("tab");
        let holes = pane_holes(tab, CONTENT, CHROME);
        let plain = pane_holes(
            layout_of("a", &[("b", "a", Edge::Right)])
                .active_tab()
                .expect("tab"),
            CONTENT,
            CHROME,
        );
        assert_eq!(holes.len(), 2, "one hole per group, not per pane");
        assert_eq!(holes[1].pane, pane_id("c"), "the shown pane");
        assert_eq!(
            holes[1].rect.y - plain[1].rect.y,
            STRIP_HEIGHT,
            "below the strip"
        );
        assert_eq!(holes[0].rect, plain[0].rect, "a lone pane has no strip");
    }
}
