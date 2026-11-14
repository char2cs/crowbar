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
use crate::split_node::{MIN_RATIO, SplitNode};
use crate::tab_id::TabId;

/// A tab: its panes arranged in a split tree, and which pane has focus.
///
/// Invariants, kept by every method: the tree's leaves are exactly the panes, each pane
/// appears once, every split has two children, and the focused pane exists. A tab without
/// panes has no tree and no focus.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct Tab {
    id: TabId,
    title: String,
    panes: BTreeMap<PaneId, PaneInstance>,
    tree: Option<SplitNode>,
    focused: Option<PaneId>,
}

impl Tab {
    /// An empty tab.
    #[must_use]
    pub fn new(id: TabId, title: impl Into<String>) -> Self {
        Self {
            id,
            title: title.into(),
            panes: BTreeMap::new(),
            tree: None,
            focused: None,
        }
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
    pub fn pane(&self, id: &PaneId) -> Option<&PaneInstance> {
        self.panes.get(id)
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

    /// The rectangle of every pane when the tab is laid out in `bounds`.
    #[must_use]
    pub fn layout(&self, bounds: Rect) -> Vec<PaneSlot> {
        let mut slots = Vec::with_capacity(self.panes.len());
        if let Some(tree) = &self.tree {
            tree.place(bounds, &mut slots);
        }
        slots
    }

    /// Renames the tab.
    pub fn set_title(&mut self, title: impl Into<String>) {
        self.title = title.into();
    }

    /// Adds a pane. With no `placement` the pane becomes the root of an empty tab, or goes to
    /// the right of the focused pane.
    ///
    /// # Errors
    /// [`Error::DuplicatePane`], [`Error::PaneNotFound`] for an unknown placement target, or
    /// [`Error::PlacementRequired`] when there is nothing to place next to.
    pub fn open_pane(
        &mut self,
        instance: PaneInstance,
        placement: Option<Placement>,
    ) -> Result<()> {
        let id = instance.pane_id.clone();
        if self.panes.contains_key(&id) {
            return Err(Error::DuplicatePane(id));
        }
        if self.tree.is_none() {
            self.tree = Some(SplitNode::leaf(id.clone()));
            self.focused = Some(id.clone());
            self.panes.insert(id, instance);
            return Ok(());
        }
        let placement = match placement {
            Some(placement) => placement,
            None => self.default_placement()?,
        };
        let split = self
            .tree
            .as_mut()
            .is_some_and(|tree| tree.split_leaf(&placement.target, &id, placement.edge));
        if !split {
            return Err(Error::PaneNotFound(placement.target));
        }
        self.focused = Some(id.clone());
        self.panes.insert(id, instance);
        Ok(())
    }

    /// Removes a pane and returns it. Focus moves to the pane that takes its space.
    ///
    /// # Errors
    /// [`Error::PaneNotFound`] when the tab has no such pane.
    pub fn close_pane(&mut self, pane: &PaneId) -> Result<PaneInstance> {
        let instance = self
            .panes
            .remove(pane)
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        let heir = self.tree.as_ref().and_then(|t| t.sibling_of(pane)).cloned();
        self.tree = self.tree.take().and_then(|tree| tree.without(pane));
        if self.focused.as_ref() == Some(pane) {
            self.focused = heir;
        }
        Ok(instance)
    }

    /// Moves a pane next to another pane of the same tab.
    ///
    /// # Errors
    /// [`Error::PaneNotFound`] for an unknown pane or target and [`Error::SamePane`] when both
    /// are the same pane.
    pub fn move_pane(&mut self, pane: &PaneId, placement: &Placement) -> Result<()> {
        if pane == &placement.target {
            return Err(Error::SamePane(pane.clone()));
        }
        self.require(pane)?;
        self.require(&placement.target)?;
        self.tree = self.tree.take().and_then(|tree| tree.without(pane));
        let placed = self
            .tree
            .as_mut()
            .is_some_and(|tree| tree.split_leaf(&placement.target, pane, placement.edge));
        debug_assert!(placed, "target was verified before the pane was lifted");
        Ok(())
    }

    /// Exchanges the positions of two panes.
    ///
    /// # Errors
    /// [`Error::PaneNotFound`] for an unknown pane and [`Error::SamePane`] when `a == b`.
    pub fn swap_panes(&mut self, a: &PaneId, b: &PaneId) -> Result<()> {
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

    /// Gives a pane focus.
    ///
    /// # Errors
    /// [`Error::PaneNotFound`] when the tab has no such pane.
    pub fn focus(&mut self, pane: &PaneId) -> Result<()> {
        self.require(pane)?;
        self.focused = Some(pane.clone());
        Ok(())
    }

    /// Replaces a pane's persisted state.
    ///
    /// # Errors
    /// [`Error::PaneNotFound`] when the tab has no such pane.
    pub fn set_pane_state(&mut self, pane: &PaneId, state: PaneState) -> Result<()> {
        let instance = self
            .panes
            .get_mut(pane)
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        instance.state = state;
        Ok(())
    }

    /// Sets the share the first child of the split at `path` receives, clamped to the
    /// allowed range.
    ///
    /// # Errors
    /// [`Error::InvalidRatio`] for a non-finite ratio and [`Error::NoSuchSplit`] when `path`
    /// does not address a split.
    pub fn resize_split(&mut self, path: &[Branch], ratio: f32) -> Result<()> {
        if !ratio.is_finite() {
            return Err(Error::InvalidRatio);
        }
        let node = self
            .tree
            .as_mut()
            .and_then(|tree| tree.split_at_mut(path))
            .ok_or(Error::NoSuchSplit)?;
        if let SplitNode::Split { ratio: own, .. } = node {
            *own = ratio.clamp(MIN_RATIO, 1.0 - MIN_RATIO);
        }
        Ok(())
    }

    pub(crate) fn states(&self) -> Vec<(PaneId, PaneState)> {
        let own = |(id, instance): (&PaneId, &PaneInstance)| (id.clone(), instance.state.clone());
        self.panes.iter().map(own).collect()
    }

    pub(crate) fn apply_states(&mut self, states: &[(PaneId, PaneState)]) {
        let newer = |instance: &PaneInstance| {
            states
                .iter()
                .find(|(id, _)| id == &instance.pane_id)
                .map(|(_, state)| state.clone())
        };
        for instance in self.panes.values_mut() {
            instance.state = newer(instance).unwrap_or_else(|| instance.state.clone());
        }
    }

    pub(crate) fn validate(&self) -> Result<()> {
        let mut leaves = self.pane_ids();
        leaves.sort();
        let keys: Vec<PaneId> = self.panes.keys().cloned().collect();
        if leaves != keys {
            return Err(Error::Invalid("tree leaves differ from the pane set"));
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
        if focus_ok {
            Ok(())
        } else {
            Err(Error::Invalid("focus does not match the panes"))
        }
    }

    fn default_placement(&self) -> Result<Placement> {
        let target = self
            .focused
            .clone()
            .ok_or(Error::PlacementRequired(self.id))?;
        Ok(Placement {
            target,
            edge: Edge::Right,
        })
    }

    fn require(&self, pane: &PaneId) -> Result<()> {
        if self.panes.contains_key(pane) {
            Ok(())
        } else {
            Err(Error::PaneNotFound(pane.clone()))
        }
    }
}

#[cfg(test)]
#[path = "tab_tests.rs"]
mod tests;
