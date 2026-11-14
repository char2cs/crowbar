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
    pub(crate) fn new(id: TabId, title: impl Into<String>) -> Self {
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

    pub(crate) fn restored(shape: TabShape, panes: BTreeMap<PaneId, PaneInstance>) -> Self {
        Self {
            id: shape.id,
            title: shape.title,
            panes,
            tree: shape.tree,
            focused: shape.focused,
        }
    }

    pub(crate) fn from_shape(shape: TabShape, pool: &mut BTreeMap<PaneId, PaneInstance>) -> Self {
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
    pub fn pane(&self, id: &PaneId) -> Option<&PaneInstance> {
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
    pub fn layout(&self, bounds: Rect) -> Vec<PaneSlot> {
        let mut slots = Vec::with_capacity(self.panes.len());
        if let Some(tree) = &self.tree {
            tree.place(bounds, &mut slots);
        }
        slots
    }

    /// The divider of every split when the tab is laid out in `bounds`, outermost first.
    #[must_use]
    pub fn sashes(&self, bounds: Rect) -> Vec<Sash> {
        let mut sashes = Vec::new();
        if let Some(tree) = &self.tree {
            tree.place_sashes(bounds, &mut Vec::new(), &mut sashes);
        }
        sashes
    }

    pub(crate) fn contains(&self, pane: &PaneId) -> bool {
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

    pub(crate) fn set_title(&mut self, title: String) {
        self.title = title;
    }

    pub(crate) fn check_open(&self, id: &PaneId, placement: Option<&Placement>) -> Result<()> {
        if self.panes.contains_key(id) {
            return Err(Error::DuplicatePane(id.clone()));
        }
        if self.panes.len() >= MAX_PANES_PER_TAB {
            return Err(Error::TooManyPanes {
                tab: self.id,
                limit: MAX_PANES_PER_TAB,
            });
        }
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

    fn insert_leaf(&mut self, id: &PaneId, placement: Option<Placement>) -> Result<()> {
        if self.tree.is_none() {
            self.tree = Some(SplitNode::leaf(id.clone()));
            return Ok(());
        }
        let placement = self.resolve(placement)?;
        let split = self
            .tree
            .as_mut()
            .is_some_and(|tree| tree.split_leaf(&placement.target, id, placement.edge));
        if split {
            Ok(())
        } else {
            Err(Error::PaneNotFound(placement.target))
        }
    }

    pub(crate) fn remove(&mut self, pane: &PaneId) -> Result<PaneInstance> {
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

    pub(crate) fn relocate(&mut self, pane: &PaneId, placement: &Placement) -> Result<()> {
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

    pub(crate) fn swap(&mut self, a: &PaneId, b: &PaneId) -> Result<()> {
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

    pub(crate) fn focus(&mut self, pane: &PaneId) -> Result<()> {
        self.require(pane)?;
        self.focused = Some(pane.clone());
        Ok(())
    }

    pub(crate) fn set_state(&mut self, pane: &PaneId, state: PaneState) -> Result<()> {
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
        if !first_extent.is_finite() || first_extent < 0.0 || !bounds.is_valid() {
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

    pub(crate) fn equalize(&mut self, path: &[Branch]) -> Result<()> {
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
        if focus_ok {
            Ok(())
        } else {
            Err(Error::Invalid("focus does not match the panes"))
        }
    }

    fn target_of(&self, placement: Option<&Placement>) -> Result<PaneId> {
        match placement {
            Some(placement) => Ok(placement.target.clone()),
            None => self
                .focused
                .clone()
                .ok_or(Error::PlacementRequired(self.id)),
        }
    }

    fn resolve(&self, placement: Option<Placement>) -> Result<Placement> {
        match placement {
            Some(placement) => Ok(placement),
            None => Ok(Placement {
                target: self.target_of(None)?,
                edge: Edge::Right,
            }),
        }
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
