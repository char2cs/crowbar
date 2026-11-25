use protocol::{PaneId, PaneState, Params};

use crate::error::{Error, Result};
use crate::ide::pane_group::PaneGroup;
use crate::layout::tab::{MAX_PANES_PER_TAB, Tab};
use crate::tree::branch::Branch;
use crate::tree::geometry::rect::Rect;
use crate::tree::placement::Placement;
use crate::tree::split_extent::{extent_of, ratio_for};
use crate::tree::split_node::{EQUAL_SHARE, SplitNode};

impl Tab {
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
        let (rest, fresh) = self.lifted(pane);
        let mut rest = rest.ok_or_else(|| Error::PaneNotFound(placement.target.clone()))?;
        if !rest.split_leaf(&placement.target, fresh, placement.edge) {
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
        self.check_swappable(a, b)?;
        let chats = self.is_chat(a);
        let Some(tree) = self.tree.as_mut() else {
            return Ok(());
        };
        if chats {
            tree.swap_groups(a, b);
            return Ok(());
        }
        swap_tabs(tree, a, b);
        if let Some(shown) = self.focused.clone().filter(|pane| pane == a || pane == b) {
            self.show(&shown);
        }
        Ok(())
    }

    pub(crate) fn focus(
        &mut self,
        pane: &PaneId,
    ) -> Result<()> {
        self.require(pane)?;
        self.show(pane);
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

    pub(crate) fn set_params(
        &mut self,
        pane: &PaneId,
        params: Params,
    ) -> Result<()> {
        let instance = self
            .panes
            .get_mut(pane)
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        instance.params = params;
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
        if !self.tree.as_ref().is_none_or(SplitNode::groups_valid) {
            return Err(Error::Invalid(
                "a group is empty, repeats a pane or shows a stranger",
            ));
        }
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
            Some(id) => self.is_shown(id),
            None => self.panes.is_empty(),
        };
        focus_ok
            .then_some(())
            .ok_or(Error::Invalid("focus does not match the panes"))
    }
}

type Restore = fn(&mut PaneGroup, &PaneId);

fn swap_tabs(
    tree: &mut SplitNode,
    a: &PaneId,
    b: &PaneId,
) {
    let pins = [a, b].map(|pane| (pane, pin_of(tree, pane)));
    tree.swap_leaves(a, b);
    keep_pins(tree, pins);
}

fn pin_of(
    tree: &SplitNode,
    pane: &PaneId,
) -> Restore {
    let pinned = tree
        .group_of(pane)
        .is_some_and(|group| group.is_pinned(pane));
    if pinned {
        return PaneGroup::pin;
    }
    PaneGroup::unpin
}

fn keep_pins(
    tree: &mut SplitNode,
    pins: [(&PaneId, Restore); 2],
) {
    for (pane, restore) in pins {
        keep_pin(tree, pane, restore);
    }
}

fn keep_pin(
    tree: &mut SplitNode,
    pane: &PaneId,
    restore: Restore,
) {
    if let Some(group) = tree.group_of_mut(pane) {
        restore(group, pane);
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::tab_id::TabId;
    use crate::test_support::tab_fixtures::{at, editor, id, order, screen, tab_with};
    use crate::tree::edge::Edge;
    use crate::tree::split_node::MIN_PANE_EXTENT;

    type Breaker = fn(&mut Tab);

    #[test]
    fn focusing_the_chat_selects_it_and_focusing_a_tab_selects_the_tab() {
        let mut tab = tab_with(&["a"]);
        tab.open(editor("t1"), None).unwrap();
        tab.focus(&id("a")).unwrap();
        let group = tab.group_of(&id("a")).unwrap();
        assert!(group.chat_selected(), "chat selected");
        assert_eq!(group.active_tab(), Some(&id("t1")), "the tab stays active");
        tab.focus(&id("t1")).unwrap();
        assert!(
            !tab.group_of(&id("a")).unwrap().chat_selected(),
            "tab selected"
        );
        tab.validate().unwrap();
    }

    #[test]
    fn moving_a_chat_takes_its_editor_tabs_along() {
        let mut tab = tab_with(&["a", "b"]);
        tab.open(editor("t1"), None).unwrap();
        tab.relocate(&id("b"), &at("a", Edge::Left)).unwrap();
        assert_eq!(order(&tab), ["b", "t1", "a"], "b and its tab left of a");
        assert_eq!(tab.group_count(), 2, "still two places");
        tab.validate().unwrap();
    }

    #[test]
    fn a_chat_and_an_editor_tab_never_swap() {
        let mut tab = tab_with(&["a"]);
        tab.open(editor("t1"), None).unwrap();
        assert_eq!(
            tab.swap(&id("a"), &id("t1")),
            Err(Error::ChatIsNotATab(id("a"))),
            "refused"
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
            Tab::new(TabId(2), "e", crate::space_id::SpaceId::default()).resize(&[], bounds, 1.0),
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
}
