use protocol::{PaneTypeId, Params};
use serde_json::json;

use super::*;
use crate::axis::Axis;
use crate::split_node::{MIN_PANE_EXTENT, SASH_THICKNESS};

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
        tab.open(pane(name), None).unwrap();
    }
    tab
}

fn screen(width: f32, height: f32) -> Rect {
    Rect {
        x: 0.0,
        y: 0.0,
        width,
        height,
    }
}

#[test]
fn first_pane_becomes_root_and_focus() {
    let tab = tab_with(&["a"]);
    assert_eq!(order(&tab), ["a"], "order");
    assert_eq!(tab.focused(), Some(&id("a")), "focus");
    assert_eq!((tab.len(), tab.is_empty()), (1, false), "size");
    tab.validate().unwrap();
}

#[test]
fn default_placement_splits_the_focused_pane_to_the_right() {
    let tab = tab_with(&["a", "b", "c"]);
    assert_eq!(order(&tab), ["a", "b", "c"], "each right of the previous");
    assert_eq!(tab.focused(), Some(&id("c")), "new pane focused");
}

#[test]
fn open_validates_before_changing_anything() {
    let mut tab = tab_with(&["a"]);
    let before = tab.clone();
    assert_eq!(
        tab.open(pane("a"), None),
        Err(Error::DuplicatePane(id("a"))),
        "duplicate"
    );
    assert_eq!(
        tab.open(pane("b"), Some(at("zz", Edge::Left))),
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
        tab.open(pane("c"), None),
        Err(Error::PlacementRequired(TabId(1))),
        "no focus"
    );
}

#[test]
fn a_tab_holds_a_bounded_number_of_panes() {
    let mut tab = Tab::new(TabId(1), "t");
    for index in 0..MAX_PANES_PER_TAB {
        tab.open(pane(&format!("p{index}")), None).unwrap();
    }
    let before = tab.clone();
    assert_eq!(
        tab.open(pane("one-more"), None),
        Err(Error::TooManyPanes {
            tab: TabId(1),
            limit: MAX_PANES_PER_TAB
        }),
        "full"
    );
    assert_eq!(tab, before, "unchanged");
    tab.validate().unwrap();
}

#[test]
fn closing_the_right_pane_of_a_nested_row_focuses_the_pane_beside_it() {
    let mut tab = Tab::new(TabId(1), "t");
    tab.open(pane("a"), None).unwrap();
    tab.open(pane("x"), Some(at("a", Edge::Right))).unwrap();
    tab.open(pane("b"), Some(at("a", Edge::Right))).unwrap();
    tab.focus(&id("x")).unwrap();
    assert_eq!(order(&tab), ["a", "b", "x"], "[[a|b]|x]");
    tab.remove(&id("x")).unwrap();
    assert_eq!(
        tab.focused(),
        Some(&id("b")),
        "b is adjacent, not the far left a"
    );
    tab.validate().unwrap();
}

#[test]
fn closing_a_pane_hands_focus_to_the_neighbour_taking_its_space() {
    let mut tab = tab_with(&["a", "b", "c"]);
    tab.focus(&id("b")).unwrap();
    let closed = tab.remove(&id("b")).unwrap();
    assert_eq!(closed.pane_id, id("b"), "returned instance");
    assert_eq!(tab.focused(), Some(&id("c")), "heir focused");
    assert_eq!(order(&tab), ["a", "c"], "order");
    tab.validate().unwrap();
}

#[test]
fn closing_an_unfocused_pane_keeps_focus() {
    let mut tab = tab_with(&["a", "b"]);
    tab.focus(&id("a")).unwrap();
    tab.remove(&id("b")).unwrap();
    assert_eq!(tab.focused(), Some(&id("a")), "focus kept");
}

#[test]
fn removing_the_last_pane_empties_the_tab() {
    let mut tab = tab_with(&["a"]);
    tab.remove(&id("a")).unwrap();
    assert!(tab.tree().is_none() && tab.focused().is_none(), "empty");
    tab.validate().unwrap();
    assert_eq!(
        tab.remove(&id("a")),
        Err(Error::PaneNotFound(id("a"))),
        "again"
    );
}

#[test]
fn relocate_lands_on_the_requested_edge() {
    let mut tab = tab_with(&["a", "b", "c"]);
    tab.relocate(&id("c"), &at("a", Edge::Left)).unwrap();
    assert_eq!(order(&tab), ["c", "a", "b"], "c now first");
    assert_eq!(tab.focused(), Some(&id("c")), "focus untouched: c had it");
    tab.validate().unwrap();
}

#[test]
fn relocate_rejects_bad_requests_without_changes() {
    let mut tab = tab_with(&["a", "b"]);
    let before = tab.clone();
    assert_eq!(
        tab.relocate(&id("a"), &at("a", Edge::Left)),
        Err(Error::SamePane(id("a"))),
        "self"
    );
    assert_eq!(
        tab.relocate(&id("a"), &at("zz", Edge::Left)),
        Err(Error::PaneNotFound(id("zz"))),
        "target"
    );
    assert_eq!(
        tab.relocate(&id("zz"), &at("a", Edge::Left)),
        Err(Error::PaneNotFound(id("zz"))),
        "pane"
    );
    assert_eq!(tab, before, "unchanged");
}

#[test]
fn swap_exchanges_positions_and_validates() {
    let mut tab = tab_with(&["a", "b", "c"]);
    tab.swap(&id("a"), &id("c")).unwrap();
    assert_eq!(order(&tab), ["c", "b", "a"], "swapped");
    assert_eq!(
        tab.swap(&id("a"), &id("a")),
        Err(Error::SamePane(id("a"))),
        "self"
    );
    assert_eq!(
        tab.swap(&id("a"), &id("q")),
        Err(Error::PaneNotFound(id("q"))),
        "unknown"
    );
}

#[test]
fn resize_follows_the_pointer_and_holds_the_pixel_minimum() {
    let mut tab = tab_with(&["a", "b"]);
    let bounds = screen(1006.0, 600.0);
    let first_width = |tab: &Tab| tab.layout(bounds).first().map(|slot| slot.rect.width);
    tab.resize(&[], bounds, 300.0).unwrap();
    assert_eq!(
        first_width(&tab),
        Some(300.0),
        "exactly where it was dragged"
    );
    tab.resize(&[], bounds, 1.0).unwrap();
    assert_eq!(
        first_width(&tab),
        Some(MIN_PANE_EXTENT),
        "dragged past the minimum"
    );
    tab.resize(&[], bounds, 5000.0).unwrap();
    assert_eq!(
        first_width(&tab),
        Some(1000.0 - MIN_PANE_EXTENT),
        "the other side keeps the minimum"
    );
    tab.validate().unwrap();
}

#[test]
fn the_committed_ratio_is_the_one_last_shown_during_the_drag() {
    let mut tab = tab_with(&["a", "b"]);
    let bounds = screen(1006.0, 600.0);
    for pointer in [100.0, 250.0, 417.0] {
        tab.resize(&[], bounds, pointer).unwrap();
    }
    let shown = tab.layout(bounds).first().map(|slot| slot.rect.width);
    let released = tab.clone();
    assert_eq!(
        released.layout(bounds).first().map(|slot| slot.rect.width),
        shown,
        "release changes nothing"
    );
    assert_eq!(shown, Some(417.0), "last pointer position");
}

#[test]
fn resizing_a_split_too_small_for_the_minimum_changes_nothing() {
    let mut tab = tab_with(&["a", "b"]);
    let before = tab.clone();
    tab.resize(&[], screen(MIN_PANE_EXTENT, 100.0), 10.0)
        .unwrap();
    assert_eq!(tab, before, "stays as it was");
}

#[test]
fn resize_rejects_bad_input() {
    let mut tab = tab_with(&["a", "b"]);
    let before = tab.clone();
    let bounds = screen(1006.0, 600.0);
    for extent in [f32::NAN, f32::INFINITY, -1.0] {
        assert_eq!(
            tab.resize(&[], bounds, extent),
            Err(Error::InvalidGeometry),
            "{extent}"
        );
    }
    assert_eq!(
        tab.resize(&[], screen(f32::NAN, 1.0), 10.0),
        Err(Error::InvalidGeometry),
        "bounds"
    );
    assert_eq!(
        tab.resize(&[Branch::First], bounds, 300.0),
        Err(Error::NoSuchSplit),
        "leaf path"
    );
    assert_eq!(tab, before, "unchanged");
    assert_eq!(
        Tab::new(TabId(2), "e").resize(&[], bounds, 1.0),
        Err(Error::NoSuchSplit),
        "no tree"
    );
}

#[test]
fn equalize_restores_equal_shares() {
    let mut tab = tab_with(&["a", "b"]);
    let bounds = screen(1006.0, 600.0);
    tab.resize(&[], bounds, 200.0).unwrap();
    tab.equalize(&[]).unwrap();
    let widths: Vec<f32> = tab.layout(bounds).iter().map(|s| s.rect.width).collect();
    assert_eq!(widths, [500.0, 500.0], "equal");
    assert_eq!(
        tab.equalize(&[Branch::Second]),
        Err(Error::NoSuchSplit),
        "leaf"
    );
}

#[test]
fn layout_and_sashes_tile_the_bounds_exactly() {
    let mut tab = tab_with(&["a", "b", "c"]);
    tab.open(pane("d"), Some(at("c", Edge::Bottom))).unwrap();
    let bounds = Rect {
        x: 10.0,
        y: 20.0,
        width: 1000.0,
        height: 800.0,
    };
    let slots = tab.layout(bounds);
    let sashes = tab.sashes(bounds);
    assert_eq!(
        (slots.len(), sashes.len()),
        (4, 3),
        "one slot per pane, one sash per split"
    );
    let area: f32 = slots.iter().map(|s| s.rect.width * s.rect.height).sum();
    let sash_area: f32 = sashes.iter().map(|s| s.rect.width * s.rect.height).sum();
    assert!(
        (area + sash_area - bounds.width * bounds.height).abs() < 1.0,
        "panes and sashes cover the bounds: {area} + {sash_area}"
    );
    assert!(
        sashes
            .iter()
            .all(|s| s.rect.width.min(s.rect.height) == SASH_THICKNESS),
        "sash thickness"
    );
    assert!(
        sashes.iter().any(|s| s.axis == Axis::Vertical),
        "nested axis"
    );
}

#[test]
fn state_updates_stay_with_the_pane() {
    let mut tab = tab_with(&["a"]);
    let state = PaneState::new(json!({"scroll": 4})).unwrap();
    tab.set_state(&id("a"), state.clone()).unwrap();
    assert_eq!(tab.pane(&id("a")).map(|p| &p.state), Some(&state), "stored");
    assert_eq!(
        tab.set_state(&id("q"), state),
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
    tab.tree.as_mut().unwrap().set_ratio(2.0);
    assert!(tab.validate().is_err(), "ratio");
}

#[test]
fn deserializing_a_tab_checks_its_invariants() {
    let tab = tab_with(&["a", "b"]);
    let mut value = serde_json::to_value(&tab).unwrap();
    assert_eq!(
        serde_json::from_value::<Tab>(value.clone()).unwrap(),
        tab,
        "round trip"
    );
    value["focused"] = json!("ghost");
    assert!(
        serde_json::from_value::<Tab>(value.clone()).is_err(),
        "bad focus"
    );
    value["focused"] = json!("a");
    value["extra"] = json!(1);
    assert!(
        serde_json::from_value::<Tab>(value).is_err(),
        "unknown field"
    );
}

#[test]
fn shapes_snapshot_the_arrangement_and_rebuild_from_a_pool() {
    let tab = tab_with(&["a", "b"]);
    let mut pool: BTreeMap<PaneId, PaneInstance> = tab
        .pane_ids()
        .into_iter()
        .filter_map(|id| tab.pane(&id).cloned().map(|p| (id, p)))
        .collect();
    let rebuilt = Tab::from_shape(tab.shape(), &mut pool);
    assert_eq!(rebuilt, tab, "same tab");
    assert!(pool.is_empty(), "instances moved out of the pool");
}
