use layout::{Axis, Branch, Command, PaneSlot, Rect, Sash, SplitNode, Tab, TabId};

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
    pub fn begin(
        tab: &Tab,
        sash: &Sash,
        pointer: (f32, f32),
        bounds: Rect,
    ) -> Option<Self> {
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
    pub fn resize(
        &self,
        pointer: (f32, f32),
    ) -> Command {
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

fn along(
    axis: Axis,
    (x, y): (f32, f32),
) -> f32 {
    match axis {
        Axis::Horizontal => x,
        Axis::Vertical => y,
    }
}

fn first_extent(
    tab: &Tab,
    sash: &Sash,
    bounds: Rect,
) -> Option<f32> {
    let node = sash
        .path
        .iter()
        .try_fold(tab.tree()?, |node, step| child(node, *step))?;
    let SplitNode::Split { first, .. } = node else {
        return None;
    };
    let slots = tab.layout(bounds);
    Some(extent_of(first, &slots, sash.axis))
}

fn child(
    node: &SplitNode,
    step: Branch,
) -> Option<&SplitNode> {
    let SplitNode::Split { first, second, .. } = node else {
        return None;
    };
    Some(match step {
        Branch::First => first,
        Branch::Second => second,
    })
}

fn extent_of(
    node: &SplitNode,
    slots: &[PaneSlot],
    axis: Axis,
) -> f32 {
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

fn leaves(
    node: &SplitNode,
    out: &mut Vec<protocol::PaneId>,
) {
    match node {
        SplitNode::Leaf(group) => out.push(group.active().clone()),
        SplitNode::Split { first, second, .. } => {
            leaves(first, out);
            leaves(second, out);
        },
    }
}

#[cfg(test)]
mod tests {
    use layout::{Command, Edge, Rect};

    use super::*;
    use crate::test_support::layout_of;

    const CONTENT: Rect = Rect {
        x: 100.0,
        y: 40.0,
        width: 806.0,
        height: 600.0,
    };

    struct Case {
        name: &'static str,
        edge: Edge,
        grab: (f32, f32),
        to: (f32, f32),
        first_extent: f32,
    }

    const CASES: &[Case] = &[
        Case {
            name: "moving the pointer by forty pixels grows the first child by forty",
            edge: Edge::Right,
            grab: (503.0, 300.0),
            to: (543.0, 310.0),
            first_extent: 440.0,
        },
        Case {
            name: "a vertical split follows the vertical pointer only",
            edge: Edge::Bottom,
            grab: (300.0, 340.0),
            to: (900.0, 300.0),
            first_extent: 297.0 - 40.0,
        },
    ];

    fn grab_at(
        layout: &layout::Layout,
        pointer: (f32, f32),
    ) -> SashGrab {
        let tab = layout.active_tab().expect("tab");
        let sash = tab.sashes(CONTENT).remove(0);
        SashGrab::begin(tab, &sash, pointer, CONTENT).expect("grab")
    }

    fn resized(case: &Case) -> (f32, Rect) {
        let layout = layout_of("a", &[("b", "a", case.edge)]);
        let grab = grab_at(&layout, case.grab);
        let Command::ResizeSplit {
            first_extent,
            bounds,
            ..
        } = grab.resize(case.to)
        else {
            panic!("a resize command: {}", case.name);
        };
        (first_extent, bounds)
    }

    #[test]
    fn the_pointer_resizes_the_first_child_along_the_split() {
        for case in CASES {
            let (first_extent, bounds) = resized(case);
            assert_eq!(first_extent, case.first_extent, "{}", case.name);
            assert_eq!(bounds, CONTENT, "{}: bounds of the tab", case.name);
        }
    }

    #[test]
    fn the_command_applies_and_moves_the_sash() {
        let mut layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let grab = grab_at(&layout, (503.0, 300.0));
        layout
            .apply(grab.resize((603.0, 300.0)))
            .expect("resize applies");
        let tab = layout.active_tab().expect("tab");
        assert_eq!(
            tab.sashes(CONTENT)[0].rect.x,
            600.0,
            "sash followed the pointer by 100px"
        );
    }

    #[test]
    fn reset_names_the_same_split() {
        let layout = layout_of("a", &[("b", "a", Edge::Right)]);
        let grab = grab_at(&layout, (503.0, 300.0));
        let Command::ResetSplit { path, .. } = grab.reset() else {
            panic!("a reset command");
        };
        assert!(path.is_empty(), "the root split");
    }
}
