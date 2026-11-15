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
    pub fn resolve(
        subject: &DragSubject,
        point: (f32, f32),
        slots: &[PaneSlot],
    ) -> Option<Self> {
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
    pub fn preview(
        &self,
        pane: Rect,
    ) -> Rect {
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
    pub fn command(
        &self,
        subject: &DragSubject,
        tab: &Tab,
    ) -> Option<Command> {
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

fn own_pane(
    subject: &DragSubject,
    pane: &PaneId,
) -> bool {
    matches!(subject, DragSubject::Pane(dragged) if dragged == pane)
}

fn contains(
    rect: Rect,
    (x, y): (f32, f32),
) -> bool {
    x >= rect.x && x < rect.x + rect.width && y >= rect.y && y < rect.y + rect.height
}

#[cfg(test)]
mod tests {
    use model::{Command, DragSubject, Edge, Placement, Rect, TabId};

    use super::*;
    use crate::test_support::{chat, layout_of, pane_id};

    const CONTENT: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 806.0,
        height: 400.0,
    };

    fn two_panes() -> model::Layout {
        layout_of("a", &[("b", "a", Edge::Right)])
    }

    #[test]
    fn a_point_over_the_left_edge_of_pane_b_targets_b_on_its_left() {
        let layout = two_panes();
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Open(chat("c"));
        let target = DropTarget::resolve(&subject, (410.0, 200.0), &slots).expect("target");
        assert_eq!(
            target,
            DropTarget {
                pane: pane_id("b"),
                edge: Edge::Left
            },
            "b left"
        );
    }

    #[test]
    fn a_pane_dropped_on_itself_has_no_target() {
        let layout = two_panes();
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Pane(pane_id("a"));
        assert_eq!(
            DropTarget::resolve(&subject, (100.0, 200.0), &slots),
            None,
            "self drop"
        );
    }

    #[test]
    fn a_tab_drag_never_targets_a_pane() {
        let layout = two_panes();
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Tab(TabId(1));
        assert_eq!(
            DropTarget::resolve(&subject, (100.0, 200.0), &slots),
            None,
            "tab drag"
        );
    }

    #[test]
    fn a_point_over_the_sash_hits_nothing() {
        let layout = two_panes();
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Open(chat("c"));
        assert_eq!(
            DropTarget::resolve(&subject, (403.0, 200.0), &slots),
            None,
            "sash gap"
        );
    }

    #[test]
    fn the_centre_of_an_open_pane_previews_its_right_half() {
        let pane = Rect {
            x: 0.0,
            y: 0.0,
            width: 400.0,
            height: 200.0,
        };
        let target = DropTarget {
            pane: pane_id("a"),
            edge: Edge::Right,
        };
        let expected = Rect {
            x: 200.0,
            y: 4.0,
            width: 196.0,
            height: 192.0,
        };
        assert_eq!(target.preview(pane), expected, "right half inset 4px");
    }

    #[test]
    fn every_edge_previews_the_half_it_will_take() {
        let pane = Rect {
            x: 0.0,
            y: 0.0,
            width: 108.0,
            height: 108.0,
        };
        let rect = |x, y, width, height| Rect {
            x,
            y,
            width,
            height,
        };
        let cases = [
            (Edge::Left, rect(4.0, 4.0, 50.0, 100.0)),
            (Edge::Top, rect(4.0, 4.0, 100.0, 50.0)),
            (Edge::Bottom, rect(4.0, 54.0, 100.0, 50.0)),
        ];
        for (edge, expected) in cases {
            assert_eq!(target_on(edge).preview(pane), expected, "{edge:?}");
        }
    }

    fn target_on(edge: Edge) -> DropTarget {
        DropTarget {
            pane: pane_id("a"),
            edge,
        }
    }

    #[test]
    fn dropping_a_chat_from_the_sidebar_opens_it_beside_the_target() {
        let layout = two_panes();
        let tab = layout.active_tab().expect("tab");
        let target = DropTarget {
            pane: pane_id("b"),
            edge: Edge::Bottom,
        };
        let command = target
            .command(&DragSubject::Open(chat("c")), tab)
            .expect("command");
        let placement = Some(Placement {
            target: pane_id("b"),
            edge: Edge::Bottom,
        });
        assert_eq!(
            command,
            Command::OpenPane {
                tab: tab.id(),
                instance: chat("c"),
                placement
            }
        );
    }

    #[test]
    fn dropping_a_pane_of_the_same_tab_moves_it() {
        let layout = two_panes();
        let tab = layout.active_tab().expect("tab");
        let target = DropTarget {
            pane: pane_id("b"),
            edge: Edge::Top,
        };
        let command = target
            .command(&DragSubject::Pane(pane_id("a")), tab)
            .expect("command");
        let placement = Placement {
            target: pane_id("b"),
            edge: Edge::Top,
        };
        assert_eq!(
            command,
            Command::MovePane {
                pane: pane_id("a"),
                placement
            }
        );
    }

    #[test]
    fn dropping_a_pane_of_another_tab_moves_it_into_this_tab() {
        let layout = two_panes();
        let tab = layout.active_tab().expect("tab");
        let target = DropTarget {
            pane: pane_id("b"),
            edge: Edge::Right,
        };
        let command = target
            .command(&DragSubject::Pane(pane_id("zzz")), tab)
            .expect("command");
        let placement = Some(Placement {
            target: pane_id("b"),
            edge: Edge::Right,
        });
        assert_eq!(
            command,
            Command::MovePaneToTab {
                pane: pane_id("zzz"),
                to: tab.id(),
                placement
            }
        );
    }

    #[test]
    fn the_commands_apply_to_the_model_and_yield_three_panes() {
        let mut layout = two_panes();
        let tab = layout.active_tab().expect("tab").clone();
        let target = DropTarget {
            pane: pane_id("b"),
            edge: Edge::Bottom,
        };
        let command = target
            .command(&DragSubject::Open(chat("c")), &tab)
            .expect("command");
        layout.apply(command).expect("applies");
        assert_eq!(layout.pane_count(), 3, "three panes after the drop");
    }
}
