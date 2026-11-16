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
    /// Whether the drop joins the pane's strip instead of splitting beside it.
    pub joins: bool,
}

impl DropTarget {
    /// The target under `point` among the pane `slots` of the active tab, or `None` when no
    /// pane is hit or the drop would do nothing: a pane dropped on itself, or a tab drag, which
    /// never splits a pane.
    #[must_use]
    pub fn resolve(
        subject: &DragSubject,
        point: (f32, f32),
        (slots, joinable): (&[PaneSlot], &[PaneId]),
    ) -> Option<Self> {
        let slot = slots.iter().find(|slot| contains(slot.rect, point))?;
        if own_pane(subject, &slot.pane) || matches!(subject, DragSubject::Tab(_)) {
            return None;
        }
        let zone = PaneZone::at(point, slot.rect);
        Some(Self {
            pane: slot.pane.clone(),
            edge: zone.edge(),
            joins: zone == PaneZone::Center && joinable.contains(&slot.pane),
        })
    }

    /// The slots a drag of `subject` is resolved against. A pane dragged out of a strip of
    /// several leaves its group's slot to the group's other members, so it can be dropped on
    /// the edges of the group it comes from; a pane alone in its group still cannot.
    #[must_use]
    pub fn slots_for(
        subject: &DragSubject,
        tab: &Tab,
        bounds: Rect,
    ) -> Vec<PaneSlot> {
        let mut slots = tab.layout(bounds);
        let DragSubject::Pane(dragged) = subject else {
            return slots;
        };
        let Some(group) = tab.group_of(dragged) else {
            return slots;
        };
        let Some(other) = group.panes().iter().find(|member| *member != dragged) else {
            return slots;
        };
        let shown = group.active();
        for slot in slots.iter_mut().filter(|slot| &slot.pane == shown) {
            slot.pane = other.clone();
        }
        slots
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
        if self.joins {
            return inner;
        }
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
    /// moves, a pane of another tab joins this one, and a chat that is not open yet opens. A chat
    /// dragged from the sidebar has no pane yet, so it yields `None`: its owner builds the pane.
    #[must_use]
    pub fn command(
        &self,
        subject: &DragSubject,
        tab: &Tab,
    ) -> Option<Command> {
        if let Some(joined) = self.join_command(subject) {
            return Some(joined);
        }
        let placement = self.placement();
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
            DragSubject::Chat(_) | DragSubject::Tab(_) => None,
        }
    }

    fn join_command(
        &self,
        subject: &DragSubject,
    ) -> Option<Command> {
        let DragSubject::Pane(pane) = subject else {
            return None;
        };
        self.joins.then(|| Command::MoveToGroup {
            pane: pane.clone(),
            onto: self.pane.clone(),
            index: None,
        })
    }

    /// The pane and side as the model's placement.
    #[must_use]
    pub fn placement(&self) -> Placement {
        Placement {
            target: self.pane.clone(),
            edge: self.edge,
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
    use crate::test_support::{chat, layout_of, pane_id, stacked_layout};

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
        let target = DropTarget::resolve(&subject, (410.0, 200.0), (&slots, &[])).expect("target");
        assert_eq!(
            target,
            DropTarget {
                pane: pane_id("b"),
                edge: Edge::Left,
                joins: false,
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
            DropTarget::resolve(&subject, (100.0, 200.0), (&slots, &[])),
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
            DropTarget::resolve(&subject, (100.0, 200.0), (&slots, &[])),
            None,
            "tab drag"
        );
    }

    #[test]
    fn a_chat_row_targets_the_pane_under_it_like_an_unopened_chat() {
        let layout = two_panes();
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Chat("t1".into());
        let target = DropTarget::resolve(&subject, (600.0, 200.0), (&slots, &[])).expect("target");
        assert_eq!(
            target,
            DropTarget {
                pane: pane_id("b"),
                edge: Edge::Right,
                joins: false,
            },
            "the centre of b adds beside it, to the right"
        );
    }

    #[test]
    fn a_point_over_the_sash_hits_nothing() {
        let layout = two_panes();
        let slots = layout.active_tab().expect("tab").layout(CONTENT);
        let subject = DragSubject::Open(chat("c"));
        assert_eq!(
            DropTarget::resolve(&subject, (403.0, 200.0), (&slots, &[])),
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
            joins: false,
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
            joins: false,
        }
    }

    #[test]
    fn dropping_a_chat_from_the_sidebar_opens_it_beside_the_target() {
        let layout = two_panes();
        let tab = layout.active_tab().expect("tab");
        let target = DropTarget {
            pane: pane_id("b"),
            edge: Edge::Bottom,
            joins: false,
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
    fn a_chat_row_has_no_pane_to_command_with_yet() {
        let layout = two_panes();
        let tab = layout.active_tab().expect("tab");
        let target = target_on(Edge::Left);
        assert_eq!(
            target.command(&DragSubject::Chat("t1".into()), tab),
            None,
            "the owner builds the pane"
        );
        assert_eq!(
            target.placement(),
            Placement {
                target: pane_id("a"),
                edge: Edge::Left
            },
            "placement"
        );
    }

    #[test]
    fn dropping_a_pane_of_the_same_tab_moves_it() {
        let layout = two_panes();
        let tab = layout.active_tab().expect("tab");
        let target = DropTarget {
            pane: pane_id("b"),
            edge: Edge::Top,
            joins: false,
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
            joins: false,
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
            joins: false,
        };
        let command = target
            .command(&DragSubject::Open(chat("c")), &tab)
            .expect("command");
        layout.apply(command).expect("applies");
        assert_eq!(layout.pane_count(), 3, "three panes after the drop");
    }

    #[test]
    fn a_tab_dragged_out_of_its_own_strip_can_be_dropped_on_that_groups_edge() {
        let layout = stacked_layout();
        let tab = layout.active_tab().expect("tab");
        let subject = DragSubject::Pane(pane_id("c"));
        let slots = DropTarget::slots_for(&subject, tab, CONTENT);
        let target = DropTarget::resolve(&subject, (410.0, 200.0), (&slots, &[])).expect("target");
        assert_eq!(
            (target.pane.clone(), target.edge),
            (pane_id("b"), Edge::Left),
            "other member"
        );
        let command = target.command(&subject, tab).expect("command");
        layout
            .clone()
            .apply(command)
            .expect("moves c out beside a stack of b");
    }

    #[test]
    fn a_pane_alone_in_its_group_still_has_no_target_on_itself() {
        let layout = stacked_layout();
        let tab = layout.active_tab().expect("tab");
        let subject = DragSubject::Pane(pane_id("a"));
        let slots = DropTarget::slots_for(&subject, tab, CONTENT);
        assert_eq!(
            DropTarget::resolve(&subject, (100.0, 200.0), (&slots, &[])),
            None,
            "own lone group"
        );
    }

    #[test]
    fn the_centre_of_a_group_that_takes_the_pane_joins_its_strip_and_previews_all_of_it() {
        let layout = stacked_layout();
        let tab = layout.active_tab().expect("tab");
        let subject = DragSubject::Pane(pane_id("a"));
        let slots = tab.layout(CONTENT);
        let target = DropTarget::resolve(&subject, (600.0, 200.0), (&slots, &[pane_id("c")]))
            .expect("target");
        assert!(target.joins, "joins");
        let command = target.command(&subject, tab).expect("command");
        assert_eq!(
            command,
            Command::MoveToGroup {
                pane: pane_id("a"),
                onto: pane_id("c"),
                index: None
            },
            "centre drop"
        );
        let pane = Rect {
            x: 0.0,
            y: 0.0,
            width: 108.0,
            height: 108.0,
        };
        assert_eq!(target.preview(pane).width, 100.0, "whole pane inset");
    }

    #[test]
    fn an_edge_drop_never_joins_even_when_the_pane_could() {
        let layout = stacked_layout();
        let tab = layout.active_tab().expect("tab");
        let subject = DragSubject::Pane(pane_id("a"));
        let slots = tab.layout(CONTENT);
        let target = DropTarget::resolve(&subject, (410.0, 200.0), (&slots, &[pane_id("c")]))
            .expect("target");
        assert!(!target.joins, "edge");
        assert!(
            matches!(
                target.command(&subject, tab),
                Some(Command::MovePane { .. })
            ),
            "edge moves"
        );
    }
}
