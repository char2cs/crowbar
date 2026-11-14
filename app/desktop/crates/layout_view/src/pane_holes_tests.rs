use model::{Edge, Rect};

use super::*;
use crate::sidebar_edge::SidebarEdge;
use crate::test_support::{layout_of, pane_id};

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
