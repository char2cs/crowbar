use model::{Axis, Branch, Command, PaneSlot, Rect, Sash, SplitNode, Tab, TabId};

/// A sash held by the pointer: where the split's first child started and where the pointer was,
/// so every move maps to the first child's new size.
#[derive(Debug, Clone, PartialEq)]
pub struct SashGrab {
    tab: TabId,
    path: Vec<Branch>,
    axis: Axis,
    bounds: Rect,
    first_extent: f32,
    pointer: f32,
}

impl SashGrab {
    /// Starts a drag of `sash` with the pointer at `pointer` (window coordinates) in a tab laid
    /// out in `bounds`.
    #[must_use]
    pub fn begin(tab: &Tab, sash: &Sash, pointer: (f32, f32), bounds: Rect) -> Option<Self> {
        let first_extent = first_extent(tab, sash, bounds)?;
        Some(Self {
            tab: tab.id(),
            path: sash.path.clone(),
            axis: sash.axis,
            bounds,
            first_extent,
            pointer: along(sash.axis, pointer),
        })
    }

    /// The model command that puts the split where the pointer is now.
    #[must_use]
    pub fn resize(&self, pointer: (f32, f32)) -> Command {
        let moved = along(self.axis, pointer) - self.pointer;
        Command::ResizeSplit {
            tab: self.tab,
            path: self.path.clone(),
            bounds: self.bounds,
            first_extent: self.first_extent + moved,
        }
    }

    /// The model command for a double click: both children share the space equally again.
    #[must_use]
    pub fn reset(&self) -> Command {
        Command::ResetSplit {
            tab: self.tab,
            path: self.path.clone(),
        }
    }
}

fn along(axis: Axis, (x, y): (f32, f32)) -> f32 {
    match axis {
        Axis::Horizontal => x,
        Axis::Vertical => y,
    }
}

fn first_extent(tab: &Tab, sash: &Sash, bounds: Rect) -> Option<f32> {
    let mut node = tab.tree()?;
    for step in &sash.path {
        let SplitNode::Split { first, second, .. } = node else {
            return None;
        };
        node = match step {
            Branch::First => first,
            Branch::Second => second,
        };
    }
    let SplitNode::Split { first, .. } = node else {
        return None;
    };
    let slots = tab.layout(bounds);
    Some(extent_of(first, &slots, sash.axis))
}

fn extent_of(node: &SplitNode, slots: &[PaneSlot], axis: Axis) -> f32 {
    let mut panes = Vec::new();
    leaves(node, &mut panes);
    let rects = slots
        .iter()
        .filter(|slot| panes.contains(&slot.pane))
        .map(|slot| slot.rect);
    let (start, end) = rects.fold((f32::MAX, f32::MIN), |(start, end), rect| match axis {
        Axis::Horizontal => (start.min(rect.x), end.max(rect.x + rect.width)),
        Axis::Vertical => (start.min(rect.y), end.max(rect.y + rect.height)),
    });
    end - start
}

fn leaves(node: &SplitNode, out: &mut Vec<protocol::PaneId>) {
    match node {
        SplitNode::Leaf { pane } => out.push(pane.clone()),
        SplitNode::Split { first, second, .. } => {
            leaves(first, out);
            leaves(second, out);
        },
    }
}

#[cfg(test)]
#[path = "sash_grab_tests.rs"]
mod tests;
