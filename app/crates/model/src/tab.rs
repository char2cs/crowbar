use std::collections::BTreeMap;

use protocol::{PaneId, PaneState};
use serde::{Deserialize, Serialize};

use crate::branch::Branch;
use crate::edge::Edge;
use crate::error::{Error, Result};
use crate::pane_instance::PaneInstance;
use crate::pane_slot::PaneSlot;
use crate::placement::Placement;
use crate::rect::Rect;
use crate::sash::Sash;
use crate::split_node::{EQUAL_SHARE, SplitNode, extent_of, ratio_for};
use crate::tab_id::TabId;
use crate::tab_record::TabRecord;
use crate::tab_shape::TabShape;

/// Most panes one tab may hold; bounds the tree depth and the cost of every tree walk.
pub const MAX_PANES_PER_TAB: usize = 64;

/// A tab: its panes arranged in a split tree, and which pane has focus.
///
/// Invariants, kept by every operation and checked on load: the tree's leaves are exactly the
/// panes, each pane appears once, every split has two children, the focused pane exists and a
/// tab without panes has no tree and no focus. A tab only changes through [`Layout::apply`].
///
/// [`Layout::apply`]: crate::Layout::apply
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", try_from = "TabRecord")]
pub struct Tab {
    id: TabId,
    title: String,
    panes: BTreeMap<PaneId, PaneInstance>,
    tree: Option<SplitNode>,
    focused: Option<PaneId>,
}

impl Tab {
    pub(crate) fn new(
        id: TabId,
        title: impl Into<String>,
    ) -> Self {
        Self::restored(
            TabShape {
                id,
                title: title.into(),
                tree: None,
                focused: None,
            },
            BTreeMap::new(),
        )
    }

    pub(crate) fn restored(
        shape: TabShape,
        panes: BTreeMap<PaneId, PaneInstance>,
    ) -> Self {
        Self {
            id: shape.id,
            title: shape.title,
            panes,
            tree: shape.tree,
            focused: shape.focused,
        }
    }

    pub(crate) fn from_shape(
        shape: TabShape,
        pool: &mut BTreeMap<PaneId, PaneInstance>,
    ) -> Self {
        let mut leaves = Vec::new();
        if let Some(tree) = &shape.tree {
            tree.collect_panes(&mut leaves);
        }
        let panes = leaves
            .into_iter()
            .filter_map(|id| pool.remove(&id).map(|instance| (id, instance)))
            .collect();
        Self::restored(shape, panes)
    }

    /// The tab's id.
    #[must_use]
    pub fn id(&self) -> TabId {
        self.id
    }

    /// The tab's title.
    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    /// The split tree, or `None` while the tab has no panes.
    #[must_use]
    pub fn tree(&self) -> Option<&SplitNode> {
        self.tree.as_ref()
    }

    /// The focused pane.
    #[must_use]
    pub fn focused(&self) -> Option<&PaneId> {
        self.focused.as_ref()
    }

    /// Looks up a pane.
    #[must_use]
    pub fn pane(
        &self,
        id: &PaneId,
    ) -> Option<&PaneInstance> {
        self.panes.get(id)
    }

    /// Number of panes.
    #[must_use]
    pub fn len(&self) -> usize {
        self.panes.len()
    }

    /// Whether the tab has no panes.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.panes.is_empty()
    }

    /// Pane ids in layout order (left to right, top to bottom).
    #[must_use]
    pub fn pane_ids(&self) -> Vec<PaneId> {
        let mut ids = Vec::with_capacity(self.panes.len());
        if let Some(tree) = &self.tree {
            tree.collect_panes(&mut ids);
        }
        ids
    }

    /// The rectangle of every pane when the tab is laid out in `bounds`. Panes are separated by
    /// [`SASH_THICKNESS`](crate::SASH_THICKNESS) and never smaller than
    /// [`MIN_PANE_EXTENT`](crate::MIN_PANE_EXTENT) unless the window is too small for that.
    #[must_use]
    pub fn layout(
        &self,
        bounds: Rect,
    ) -> Vec<PaneSlot> {
        let mut slots = Vec::with_capacity(self.panes.len());
        if let Some(tree) = &self.tree {
            tree.place_panes(bounds, &mut slots);
        }
        slots
    }

    /// The divider of every split when the tab is laid out in `bounds`, outermost first.
    #[must_use]
    pub fn sashes(
        &self,
        bounds: Rect,
    ) -> Vec<Sash> {
        let mut sashes = Vec::new();
        if let Some(tree) = &self.tree {
            tree.place_sashes(bounds, &mut Vec::new(), &mut sashes);
        }
        sashes
    }

    pub(crate) fn contains(
        &self,
        pane: &PaneId,
    ) -> bool {
        self.panes.contains_key(pane)
    }

    pub(crate) fn instances(&self) -> impl Iterator<Item = &PaneInstance> {
        self.panes.values()
    }

    pub(crate) fn take_instances(&mut self) -> BTreeMap<PaneId, PaneInstance> {
        std::mem::take(&mut self.panes)
    }

    pub(crate) fn shape(&self) -> TabShape {
        TabShape {
            id: self.id,
            title: self.title.clone(),
            tree: self.tree.clone(),
            focused: self.focused.clone(),
        }
    }

    pub(crate) fn set_title(
        &mut self,
        title: String,
    ) {
        self.title = title;
    }

    pub(crate) fn check_open(
        &self,
        id: &PaneId,
        placement: Option<&Placement>,
    ) -> Result<()> {
        if self.panes.contains_key(id) {
            return Err(Error::DuplicatePane(id.clone()));
        }
        (self.panes.len() < MAX_PANES_PER_TAB)
            .then_some(())
            .ok_or(Error::TooManyPanes {
                tab: self.id,
                limit: MAX_PANES_PER_TAB,
            })?;
        if self.tree.is_none() {
            return Ok(());
        }
        let target = self.target_of(placement)?;
        self.require(&target)
    }

    pub(crate) fn open(
        &mut self,
        instance: PaneInstance,
        placement: Option<Placement>,
    ) -> Result<()> {
        let id = instance.pane_id.clone();
        self.check_open(&id, placement.as_ref())?;
        self.insert_leaf(&id, placement)?;
        self.focused = Some(id.clone());
        self.panes.insert(id, instance);
        Ok(())
    }

    fn insert_leaf(
        &mut self,
        id: &PaneId,
        placement: Option<Placement>,
    ) -> Result<()> {
        if self.tree.is_none() {
            self.tree = Some(SplitNode::leaf(id.clone()));
            return Ok(());
        }
        let placement = self.resolve(placement)?;
        let split = self
            .tree
            .as_mut()
            .is_some_and(|tree| tree.split_leaf(&placement.target, id, placement.edge));
        split
            .then_some(())
            .ok_or(Error::PaneNotFound(placement.target))
    }

    pub(crate) fn take_pane(
        &mut self,
        pane: &PaneId,
    ) -> Result<PaneInstance> {
        self.require(pane)?;
        let heir = self
            .tree
            .as_ref()
            .and_then(|tree| tree.neighbour_of(pane))
            .cloned();
        let instance = self
            .panes
            .remove(pane)
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        self.tree = self.tree.take().and_then(|tree| tree.without(pane));
        if self.focused.as_ref() == Some(pane) {
            self.focused = heir;
        }
        Ok(instance)
    }

    pub(crate) fn relocate(
        &mut self,
        pane: &PaneId,
        placement: &Placement,
    ) -> Result<()> {
        if pane == &placement.target {
            return Err(Error::SamePane(pane.clone()));
        }
        self.require(pane)?;
        self.require(&placement.target)?;
        let mut rest = self
            .tree
            .clone()
            .and_then(|tree| tree.without(pane))
            .ok_or_else(|| Error::PaneNotFound(placement.target.clone()))?;
        if !rest.split_leaf(&placement.target, pane, placement.edge) {
            return Err(Error::PaneNotFound(placement.target.clone()));
        }
        self.tree = Some(rest);
        Ok(())
    }

    pub(crate) fn swap(
        &mut self,
        a: &PaneId,
        b: &PaneId,
    ) -> Result<()> {
        if a == b {
            return Err(Error::SamePane(a.clone()));
        }
        self.require(a)?;
        self.require(b)?;
        if let Some(tree) = self.tree.as_mut() {
            tree.swap_leaves(a, b);
        }
        Ok(())
    }

    pub(crate) fn focus(
        &mut self,
        pane: &PaneId,
    ) -> Result<()> {
        self.require(pane)?;
        self.focused = Some(pane.clone());
        Ok(())
    }

    pub(crate) fn set_state(
        &mut self,
        pane: &PaneId,
        state: PaneState,
    ) -> Result<()> {
        let instance = self
            .panes
            .get_mut(pane)
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        instance.state = state;
        Ok(())
    }

    pub(crate) fn resize(
        &mut self,
        path: &[Branch],
        bounds: Rect,
        first_extent: f32,
    ) -> Result<()> {
        let is_invalid = !first_extent.is_finite() || first_extent < 0.0 || !bounds.is_valid();
        if is_invalid {
            return Err(Error::InvalidGeometry);
        }
        let tree = self.tree.as_mut().ok_or(Error::NoSuchSplit)?;
        let (axis, rect) = tree.split_rect(path, bounds).ok_or(Error::NoSuchSplit)?;
        let ratio = ratio_for(extent_of(rect, axis), first_extent);
        let node = tree.split_at_mut(path).ok_or(Error::NoSuchSplit)?;
        if let Some(ratio) = ratio {
            node.set_ratio(ratio);
        }
        Ok(())
    }

    pub(crate) fn equalize(
        &mut self,
        path: &[Branch],
    ) -> Result<()> {
        let node = self
            .tree
            .as_mut()
            .and_then(|tree| tree.split_at_mut(path))
            .ok_or(Error::NoSuchSplit)?;
        node.set_ratio(EQUAL_SHARE);
        Ok(())
    }

    pub(crate) fn validate(&self) -> Result<()> {
        let mut leaves = self.pane_ids();
        leaves.sort();
        let keys: Vec<PaneId> = self.panes.keys().cloned().collect();
        if leaves != keys {
            return Err(Error::Invalid("tree leaves differ from the pane set"));
        }
        if self.panes.len() > MAX_PANES_PER_TAB {
            return Err(Error::Invalid("too many panes in a tab"));
        }
        if self.panes.iter().any(|(key, pane)| key != &pane.pane_id) {
            return Err(Error::Invalid("pane stored under another id"));
        }
        if !self.tree.as_ref().is_none_or(SplitNode::ratios_valid) {
            return Err(Error::Invalid("split ratio out of range"));
        }
        let focus_ok = match &self.focused {
            Some(id) => self.panes.contains_key(id),
            None => self.panes.is_empty(),
        };
        focus_ok
            .then_some(())
            .ok_or(Error::Invalid("focus does not match the panes"))
    }

    fn target_of(
        &self,
        placement: Option<&Placement>,
    ) -> Result<PaneId> {
        match placement {
            Some(placement) => Ok(placement.target.clone()),
            None => self
                .focused
                .clone()
                .ok_or(Error::PlacementRequired(self.id)),
        }
    }

    fn resolve(
        &self,
        placement: Option<Placement>,
    ) -> Result<Placement> {
        match placement {
            Some(placement) => Ok(placement),
            None => Ok(Placement {
                target: self.target_of(None)?,
                edge: Edge::Right,
            }),
        }
    }

    fn require(
        &self,
        pane: &PaneId,
    ) -> Result<()> {
        self.panes
            .contains_key(pane)
            .then_some(())
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))
    }
}

impl TryFrom<TabRecord> for Tab {
    type Error = Error;

    fn try_from(record: TabRecord) -> std::result::Result<Self, Error> {
        let shape = TabShape {
            id: record.id,
            title: record.title,
            tree: record.tree,
            focused: record.focused,
        };
        let tab = Self::restored(shape, record.panes);
        tab.validate()?;
        Ok(tab)
    }
}

#[cfg(test)]
mod tests {
    use protocol::{PaneTypeId, Params};
    use serde_json::json;

    use super::*;
    use crate::axis::Axis;
    use crate::split_node::{MIN_PANE_EXTENT, SASH_THICKNESS};

    type Breaker = fn(&mut Tab);

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

    fn at(
        target: &str,
        edge: Edge,
    ) -> Placement {
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

    fn screen(
        width: f32,
        height: f32,
    ) -> Rect {
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
        let cases = [
            ("duplicate", pane("a"), None, Error::DuplicatePane(id("a"))),
            (
                "unknown target",
                pane("b"),
                Some(at("zz", Edge::Left)),
                Error::PaneNotFound(id("zz")),
            ),
        ];
        for (name, instance, placement, error) in cases {
            let mut tab = tab_with(&["a"]);
            let before = tab.clone();
            assert_eq!(tab.open(instance, placement), Err(error), "{name}");
            assert_eq!(tab, before, "{name}: unchanged after refusal");
        }
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
        tab.take_pane(&id("x")).unwrap();
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
        let closed = tab.take_pane(&id("b")).unwrap();
        assert_eq!(closed.pane_id, id("b"), "returned instance");
        assert_eq!(tab.focused(), Some(&id("c")), "heir focused");
        assert_eq!(order(&tab), ["a", "c"], "order");
        tab.validate().unwrap();
    }

    #[test]
    fn closing_an_unfocused_pane_keeps_focus() {
        let mut tab = tab_with(&["a", "b"]);
        tab.focus(&id("a")).unwrap();
        tab.take_pane(&id("b")).unwrap();
        assert_eq!(tab.focused(), Some(&id("a")), "focus kept");
    }

    #[test]
    fn removing_the_last_pane_empties_the_tab() {
        let mut tab = tab_with(&["a"]);
        tab.take_pane(&id("a")).unwrap();
        assert!(tab.tree().is_none() && tab.focused().is_none(), "empty");
        tab.validate().unwrap();
        assert_eq!(
            tab.take_pane(&id("a")),
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
        let cases = [
            ("self", "a", at("a", Edge::Left), Error::SamePane(id("a"))),
            (
                "target",
                "a",
                at("zz", Edge::Left),
                Error::PaneNotFound(id("zz")),
            ),
            (
                "pane",
                "zz",
                at("a", Edge::Left),
                Error::PaneNotFound(id("zz")),
            ),
        ];
        for (name, moved, placement, error) in cases {
            let mut tab = tab_with(&["a", "b"]);
            let before = tab.clone();
            assert_eq!(tab.relocate(&id(moved), &placement), Err(error), "{name}");
            assert_eq!(tab, before, "{name}: unchanged");
        }
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
        let cases = [
            ("exactly where it was dragged", 300.0, 300.0),
            ("dragged past the minimum", 1.0, MIN_PANE_EXTENT),
            (
                "the other side keeps the minimum",
                5000.0,
                1000.0 - MIN_PANE_EXTENT,
            ),
        ];
        for (name, pointer, expected) in cases {
            tab.resize(&[], bounds, pointer).unwrap();
            assert_eq!(first_width(&tab), Some(expected), "{name}");
        }
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
        let bounds = screen(1006.0, 600.0);
        let nan_bounds = screen(f32::NAN, 1.0);
        let cases = [
            (
                "NaN extent",
                &[][..],
                bounds,
                f32::NAN,
                Error::InvalidGeometry,
            ),
            (
                "infinite extent",
                &[],
                bounds,
                f32::INFINITY,
                Error::InvalidGeometry,
            ),
            ("negative extent", &[], bounds, -1.0, Error::InvalidGeometry),
            ("bounds", &[], nan_bounds, 10.0, Error::InvalidGeometry),
            (
                "leaf path",
                &[Branch::First],
                bounds,
                300.0,
                Error::NoSuchSplit,
            ),
        ];
        for (name, path, area, extent, error) in cases {
            let mut tab = tab_with(&["a", "b"]);
            let before = tab.clone();
            assert_eq!(tab.resize(path, area, extent), Err(error), "{name}");
            assert_eq!(tab, before, "{name}: unchanged");
        }
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
        let cases: [(&str, Breaker); 3] = [
            ("focus on a missing pane", |tab| {
                tab.focused = Some(id("zz"));
            }),
            ("orphan leaf", |tab| {
                tab.panes.remove(&id("b"));
            }),
            ("ratio", |tab| {
                tab.tree.as_mut().unwrap().set_ratio(2.0);
            }),
        ];
        for (name, break_it) in cases {
            let mut tab = tab_with(&["a", "b"]);
            break_it(&mut tab);
            assert!(tab.validate().is_err(), "{name}");
        }
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
}
