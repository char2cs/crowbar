use protocol::{PaneTypeId, Params};
use serde_json::json;

use super::*;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

fn pane(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: id(name),
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default(),
        state: PaneState::default(),
    }
}

fn at(target: &str, edge: Edge) -> Placement {
    Placement {
        target: id(target),
        edge,
    }
}

fn order(tab: &Tab) -> Vec<String> {
    tab.pane_ids().iter().map(ToString::to_string).collect()
}

fn tab_with(names: &[&str]) -> Tab {
    let mut tab = Tab::new(TabId(1), "t");
    for name in names {
        tab.open_pane(pane(name), None).unwrap();
    }
    tab
}

#[test]
fn first_pane_becomes_root_and_focus() {
    let tab = tab_with(&["a"]);
    assert_eq!(order(&tab), ["a"], "order");
    assert_eq!(tab.focused(), Some(&id("a")), "focus");
    assert!(tab.validate().is_ok(), "valid");
}

#[test]
fn default_placement_splits_the_focused_pane_to_the_right() {
    let tab = tab_with(&["a", "b", "c"]);
    assert_eq!(
        order(&tab),
        ["a", "b", "c"],
        "each opened right of the previous focus"
    );
    assert_eq!(tab.focused(), Some(&id("c")), "new pane focused");
}

#[test]
fn open_pane_validates_before_changing_anything() {
    let mut tab = tab_with(&["a"]);
    let before = tab.clone();
    assert_eq!(
        tab.open_pane(pane("a"), None),
        Err(Error::DuplicatePane(id("a"))),
        "duplicate"
    );
    assert_eq!(
        tab.open_pane(pane("b"), Some(at("zz", Edge::Left))),
        Err(Error::PaneNotFound(id("zz"))),
        "unknown target"
    );
    assert_eq!(tab, before, "unchanged after refusals");
}

#[test]
fn open_without_focus_needs_a_placement() {
    let mut tab = tab_with(&["a", "b"]);
    tab.focused = None;
    assert_eq!(
        tab.open_pane(pane("c"), None),
        Err(Error::PlacementRequired(TabId(1))),
        "no focus"
    );
}

#[test]
fn closing_moves_focus_to_the_pane_that_takes_the_space() {
    let mut tab = tab_with(&["a", "b", "c"]);
    tab.focus(&id("b")).unwrap();
    let closed = tab.close_pane(&id("b")).unwrap();
    assert_eq!(closed.pane_id, id("b"), "returned instance");
    assert_eq!(tab.focused(), Some(&id("c")), "heir focused");
    assert_eq!(order(&tab), ["a", "c"], "order");
    assert!(tab.validate().is_ok(), "valid");
}

#[test]
fn closing_an_unfocused_pane_keeps_focus() {
    let mut tab = tab_with(&["a", "b"]);
    tab.focus(&id("a")).unwrap();
    tab.close_pane(&id("b")).unwrap();
    assert_eq!(tab.focused(), Some(&id("a")), "focus kept");
}

#[test]
fn closing_the_last_pane_empties_the_tab() {
    let mut tab = tab_with(&["a"]);
    tab.close_pane(&id("a")).unwrap();
    assert!(tab.tree().is_none() && tab.focused().is_none(), "empty");
    assert!(tab.validate().is_ok(), "valid");
    assert_eq!(
        tab.close_pane(&id("a")),
        Err(Error::PaneNotFound(id("a"))),
        "again"
    );
}

#[test]
fn move_pane_lands_on_the_requested_edge() {
    let mut tab = tab_with(&["a", "b", "c"]);
    tab.move_pane(
        &id("c"),
        &Placement {
            target: id("a"),
            edge: Edge::Left,
        },
    )
    .unwrap();
    assert_eq!(order(&tab), ["c", "a", "b"], "c now first");
    assert!(tab.validate().is_ok(), "valid");
}

#[test]
fn move_pane_rejects_bad_requests_without_changes() {
    let mut tab = tab_with(&["a", "b"]);
    let before = tab.clone();
    let onto_self = Placement {
        target: id("a"),
        edge: Edge::Left,
    };
    assert_eq!(
        tab.move_pane(&id("a"), &onto_self),
        Err(Error::SamePane(id("a"))),
        "self"
    );
    let unknown = Placement {
        target: id("zz"),
        edge: Edge::Left,
    };
    assert_eq!(
        tab.move_pane(&id("a"), &unknown),
        Err(Error::PaneNotFound(id("zz"))),
        "target"
    );
    let to_a = Placement {
        target: id("a"),
        edge: Edge::Left,
    };
    assert_eq!(
        tab.move_pane(&id("zz"), &to_a),
        Err(Error::PaneNotFound(id("zz"))),
        "pane"
    );
    assert_eq!(tab, before, "unchanged");
}

#[test]
fn swap_exchanges_positions_and_validates() {
    let mut tab = tab_with(&["a", "b", "c"]);
    tab.swap_panes(&id("a"), &id("c")).unwrap();
    assert_eq!(order(&tab), ["c", "b", "a"], "swapped");
    assert_eq!(
        tab.swap_panes(&id("a"), &id("a")),
        Err(Error::SamePane(id("a"))),
        "self"
    );
    assert_eq!(
        tab.swap_panes(&id("a"), &id("q")),
        Err(Error::PaneNotFound(id("q"))),
        "unknown"
    );
}

#[test]
fn resize_clamps_and_rejects_bad_input() {
    let mut tab = tab_with(&["a", "b"]);
    tab.resize_split(&[], 0.9).unwrap();
    tab.resize_split(&[], 0.0).unwrap();
    assert!(
        matches!(tab.tree(), Some(SplitNode::Split { ratio, .. }) if *ratio == MIN_RATIO),
        "clamped"
    );
    assert_eq!(
        tab.resize_split(&[], f32::NAN),
        Err(Error::InvalidRatio),
        "nan"
    );
    assert_eq!(
        tab.resize_split(&[Branch::First], 0.5),
        Err(Error::NoSuchSplit),
        "leaf path"
    );
    assert!(tab.validate().is_ok(), "valid");
}

#[test]
fn layout_tiles_the_bounds_exactly() {
    let tab = tab_with(&["a", "b", "c", "d"]);
    let bounds = Rect {
        x: 10.0,
        y: 20.0,
        width: 400.0,
        height: 300.0,
    };
    let slots = tab.layout(bounds);
    assert_eq!(slots.len(), 4, "one slot per pane");
    let area: f32 = slots.iter().map(|s| s.rect.width * s.rect.height).sum();
    assert!((area - 400.0 * 300.0).abs() < 1.0, "areas add up: {area}");
}

#[test]
fn state_updates_stay_with_the_pane() {
    let mut tab = tab_with(&["a"]);
    let state = PaneState::new(json!({"scroll": 4})).unwrap();
    tab.set_pane_state(&id("a"), state.clone()).unwrap();
    assert_eq!(tab.pane(&id("a")).map(|p| &p.state), Some(&state), "stored");
    assert_eq!(
        tab.set_pane_state(&id("q"), state),
        Err(Error::PaneNotFound(id("q"))),
        "unknown"
    );
}

#[test]
fn validate_names_broken_invariants() {
    let mut tab = tab_with(&["a", "b"]);
    tab.focused = Some(id("zz"));
    assert!(tab.validate().is_err(), "focus on a missing pane");
    let mut tab = tab_with(&["a", "b"]);
    tab.panes.remove(&id("b"));
    assert!(tab.validate().is_err(), "orphan leaf");
    let mut tab = tab_with(&["a", "b"]);
    tab.resize_split(&[], 0.5).unwrap();
    if let Some(SplitNode::Split { ratio, .. }) = tab.tree.as_mut() {
        *ratio = 2.0;
    }
    assert!(tab.validate().is_err(), "ratio");
}
