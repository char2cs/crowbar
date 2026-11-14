use model::{Command, DragSubject, Edge, PaneSlot, Placement, Rect, Tab};
use protocol::PaneId;

use crate::pane_zone::PaneZone;

const PREVIEW_INSET: f32 = 4.0;

/// The pane and side a drag would land on if it were released now.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DropTarget {
    /// The pane the dragged chat lands next to.
    pub pane: PaneId,
    /// The side of that pane.
    pub edge: Edge,
}

impl DropTarget {
    /// The target under `point` among the pane `slots` of the active tab, or `None` when no
    /// pane is hit or the drop would do nothing: a pane dropped on itself, or a tab drag, which
    /// never splits a pane.
    #[must_use]
    pub fn resolve(subject: &DragSubject, point: (f32, f32), slots: &[PaneSlot]) -> Option<Self> {
        let slot = slots.iter().find(|slot| contains(slot.rect, point))?;
        if own_pane(subject, &slot.pane) || matches!(subject, DragSubject::Tab(_)) {
            return None;
        }
        Some(Self {
            pane: slot.pane.clone(),
            edge: PaneZone::at(point, slot.rect).edge(),
        })
    }

    /// The highlighted rectangle inside a pane occupying `pane`: the half the new chat will
    /// take, inset 4px from the pane.
    #[must_use]
    pub fn preview(&self, pane: Rect) -> Rect {
        let inner = Rect {
            x: pane.x + PREVIEW_INSET,
            y: pane.y + PREVIEW_INSET,
            width: pane.width - 2.0 * PREVIEW_INSET,
            height: pane.height - 2.0 * PREVIEW_INSET,
        };
        let (half_width, half_height) = (inner.width / 2.0, inner.height / 2.0);
        match self.edge {
            Edge::Left => Rect {
                width: half_width,
                ..inner
            },
            Edge::Right => Rect {
                x: inner.x + half_width,
                width: half_width,
                ..inner
            },
            Edge::Top => Rect {
                height: half_height,
                ..inner
            },
            Edge::Bottom => Rect {
                y: inner.y + half_height,
                height: half_height,
                ..inner
            },
        }
    }

    /// The model command that performs the drop of `subject` into `tab`: a pane of the same tab
    /// moves, a pane of another tab joins this one, and a chat that is not open yet opens.
    #[must_use]
    pub fn command(&self, subject: &DragSubject, tab: &Tab) -> Option<Command> {
        let placement = Placement {
            target: self.pane.clone(),
            edge: self.edge,
        };
        match subject {
            DragSubject::Pane(pane) if tab.pane(pane).is_some() => Some(Command::MovePane {
                pane: pane.clone(),
                placement,
            }),
            DragSubject::Pane(pane) => Some(Command::MovePaneToTab {
                pane: pane.clone(),
                to: tab.id(),
                placement: Some(placement),
            }),
            DragSubject::Open(instance) => Some(Command::OpenPane {
                tab: tab.id(),
                instance: instance.clone(),
                placement: Some(placement),
            }),
            DragSubject::Tab(_) => None,
        }
    }
}

fn own_pane(subject: &DragSubject, pane: &PaneId) -> bool {
    matches!(subject, DragSubject::Pane(dragged) if dragged == pane)
}

fn contains(rect: Rect, (x, y): (f32, f32)) -> bool {
    x >= rect.x && x < rect.x + rect.width && y >= rect.y && y < rect.y + rect.height
}

#[cfg(test)]
#[path = "drop_target_tests.rs"]
mod tests;
