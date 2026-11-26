use std::collections::BTreeMap;

use protocol::PaneId;

use crate::layout::tab::Tab;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::tab::tab_shape::TabShape;
use crate::tree::geometry::pane_slot::PaneSlot;
use crate::tree::geometry::rect::Rect;
use crate::tree::geometry::sash::Sash;

impl Tab {
    /// The rectangle of every pane when the tab is laid out in `bounds`. Panes are separated by
    /// a sash and keep a minimum size unless the window is too small for that.
    #[must_use]
    pub fn layout(
        &self,
        bounds: Rect,
    ) -> Vec<PaneSlot> {
        self.tree
            .as_ref()
            .map_or_else(Vec::new, |tree| tree.place_panes(bounds))
    }

    /// The divider of every split when the tab is laid out in `bounds`, outermost first.
    #[must_use]
    pub fn sashes(
        &self,
        bounds: Rect,
    ) -> Vec<Sash> {
        self.tree
            .as_ref()
            .map_or_else(Vec::new, |tree| tree.place_sashes(bounds, &[]))
    }

    /// The space the tab was made in; empty while a tab stored before spaces waits for one.
    #[must_use]
    pub fn space(&self) -> &str {
        self.space.as_str()
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
            space: self.space.clone(),
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
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::tab_fixtures::{at, pane, tab_with};
    use crate::tree::axis::Axis;
    use crate::tree::edge::Edge;
    use crate::tree::split_node::SASH_THICKNESS;

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
}
