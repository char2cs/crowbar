use layout::{Command, DragSubject, Edge, PaneSlot, Placement, Rect, Tab};
use protocol::PaneId;

use crate::layout_view::drag::pane_zone::PaneZone;

const PREVIEW_INSET: f32 = 4.0;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) struct DropTarget {
    pub(crate) pane: PaneId,
    pub(crate) zone: PaneZone,
    pub(crate) joins: bool,
}

impl DropTarget {
    pub(crate) fn resolve(
        subject: &DragSubject,
        point: (f32, f32),
        slots: &[PaneSlot],
        joinable: &[PaneId],
    ) -> Option<Self> {
        let slot = slots.iter().find(|slot| contains(slot.rect, point))?;
        if own_pane(subject, &slot.pane) {
            return None;
        }
        let zone = match subject {
            DragSubject::File(_) => PaneZone::Center,
            _ => PaneZone::at(point, slot.rect),
        };
        Some(Self {
            pane: slot.pane.clone(),
            zone,
            joins: zone == PaneZone::Center && joinable.contains(&slot.pane),
        })
    }

    pub(crate) fn slots_for(
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
        let Some(other) = group.tabs().iter().find(|member| *member != dragged) else {
            return slots;
        };
        let shown = group.shown();
        for slot in slots.iter_mut().filter(|slot| Some(&slot.pane) == shown) {
            slot.pane = other.clone();
        }
        slots
    }

    pub(crate) fn preview(
        &self,
        pane: Rect,
    ) -> Rect {
        let inner = Rect {
            x: pane.x + PREVIEW_INSET,
            y: pane.y + PREVIEW_INSET,
            width: pane.width - 2.0 * PREVIEW_INSET,
            height: pane.height - 2.0 * PREVIEW_INSET,
        };
        if self.zone == PaneZone::Center {
            return inner;
        }
        let (half_width, half_height) = (inner.width / 2.0, inner.height / 2.0);
        match self.zone.edge() {
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

    pub(crate) fn command(
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
            DragSubject::Chat(_) | DragSubject::Tab(_) | DragSubject::File(_) => None,
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

    pub(crate) fn placement(&self) -> Placement {
        Placement {
            target: self.pane.clone(),
            edge: self.zone.edge(),
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
    use layout::{Command, DragSubject, Edge, Placement, Rect, TabId};

    use super::*;
    use crate::test_support::{chat, layout_of, pane_id, stacked_layout};

    const CONTENT: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 806.0,
        height: 400.0,
    };

    fn two_panes() -> layout::Layout {
        layout_of("a", &[("b", "a", Edge::Right)])
    }

    fn target(
        pane: &str,
        zone: PaneZone,
    ) -> DropTarget {
        DropTarget {
            pane: pane_id(pane),
            zone,
            joins: false,
        }
    }

    fn subject(code: &str) -> DragSubject {
        let (kind, name) = code.split_once(':').expect("kind:name");
        match kind {
            "open" => DragSubject::Open(chat(name)),
            "pane" => DragSubject::Pane(pane_id(name)),
            "tab" => DragSubject::Tab(TabId(name.parse().expect("a tab id"))),
            "chat" => DragSubject::Chat(name.into()),
            _ => DragSubject::File(name.into()),
        }
    }

    type Resolve = (&'static str, f32, &'static [&'static str], Found);
    type Found = Option<(&'static str, PaneZone, bool)>;

    const BESIDE: &[Resolve] = &[
        ("open:c", 410.0, &[], Some(("b", PaneZone::Left, false))),
        ("pane:a", 100.0, &[], None),
        ("tab:2", 100.0, &[], Some(("a", PaneZone::Center, false))),
        ("chat:t1", 600.0, &[], Some(("b", PaneZone::Center, false))),
        (
            "file:src/a.rs",
            410.0,
            &[],
            Some(("b", PaneZone::Center, false)),
        ),
        ("open:c", 403.0, &[], None),
    ];

    /// In the stacked layout a tab dragged out of a strip of several can land on its own
    /// group's edge, and only a centre drop joins a strip.
    const STACKED: &[Resolve] = &[
        ("pane:c", 410.0, &[], Some(("b", PaneZone::Left, false))),
        ("pane:a", 100.0, &[], None),
        ("pane:a", 600.0, &["c"], Some(("c", PaneZone::Center, true))),
        ("pane:a", 410.0, &["c"], Some(("c", PaneZone::Left, false))),
    ];

    fn resolved(
        layout: &layout::Layout,
        case: &Resolve,
    ) -> (Option<DropTarget>, Option<DropTarget>) {
        let (code, x, joinable, expected) = case;
        let tab = layout.showing("").expect("tab");
        let subject = subject(code);
        let slots = DropTarget::slots_for(&subject, tab, CONTENT);
        let joinable: Vec<_> = joinable.iter().map(|name| pane_id(name)).collect();
        let found = DropTarget::resolve(&subject, (*x, 200.0), &slots, &joinable);
        let expected = expected.map(|(pane, zone, joins)| DropTarget {
            pane: pane_id(pane),
            zone,
            joins,
        });
        (found, expected)
    }

    #[test]
    fn the_pointer_resolves_to_the_pane_and_zone_under_it() {
        let (beside, stacked) = (two_panes(), stacked_layout());
        let beside = BESIDE.iter().map(|case| (&beside, case));
        for (layout, case) in beside.chain(STACKED.iter().map(|case| (&stacked, case))) {
            let (found, expected) = resolved(layout, case);
            assert_eq!(found, expected, "{} at {}", case.0, case.1);
        }
    }

    #[test]
    fn every_zone_previews_the_part_of_the_pane_it_will_take() {
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
            (PaneZone::Center, rect(4.0, 4.0, 100.0, 100.0)),
            (PaneZone::Left, rect(4.0, 4.0, 50.0, 100.0)),
            (PaneZone::Right, rect(54.0, 4.0, 50.0, 100.0)),
            (PaneZone::Top, rect(4.0, 4.0, 100.0, 50.0)),
            (PaneZone::Bottom, rect(4.0, 54.0, 100.0, 50.0)),
        ];
        for (zone, expected) in cases {
            assert_eq!(target("a", zone).preview(pane), expected, "{zone:?}");
        }
    }

    fn beside_b(edge: Edge) -> Placement {
        Placement {
            target: pane_id("b"),
            edge,
        }
    }

    #[test]
    fn a_dropped_chat_opens_beside_the_target_unless_its_owner_must_build_its_pane() {
        let layout = two_panes();
        let tab = layout.showing("").expect("tab");
        let open = Command::OpenPane {
            tab: tab.id(),
            instance: chat("c"),
            placement: Some(beside_b(Edge::Right)),
        };
        let cases = [
            ("open:c", PaneZone::Center, Some(open)),
            ("chat:t1", PaneZone::Left, None),
        ];
        for (code, zone, expected) in cases {
            assert_eq!(
                target("b", zone).command(&subject(code), tab),
                expected,
                "{code}"
            );
        }
    }

    #[test]
    fn a_dropped_pane_moves_within_the_tab_into_it_or_into_the_target_strip() {
        let layout = two_panes();
        let tab = layout.showing("").expect("tab");
        let within = Command::MovePane {
            pane: pane_id("a"),
            placement: beside_b(Edge::Top),
        };
        let into = Command::MovePaneToTab {
            pane: pane_id("zzz"),
            to: tab.id(),
            placement: Some(beside_b(Edge::Right)),
        };
        let joined = Command::MoveToGroup {
            pane: pane_id("a"),
            onto: pane_id("b"),
            index: None,
        };
        let cases = [
            ("pane:a", target("b", PaneZone::Top), within),
            ("pane:zzz", target("b", PaneZone::Right), into),
            (
                "pane:a",
                DropTarget {
                    joins: true,
                    ..target("b", PaneZone::Center)
                },
                joined,
            ),
        ];
        for (code, target, expected) in cases {
            assert_eq!(
                target.command(&subject(code), tab),
                Some(expected),
                "{code}"
            );
        }
    }

    #[test]
    fn a_tab_dragged_out_of_its_strip_lands_beside_its_old_group() {
        let layout = stacked_layout();
        let tab = layout.showing("").expect("tab");
        let subject = DragSubject::Pane(pane_id("c"));
        let slots = DropTarget::slots_for(&subject, tab, CONTENT);
        let found = DropTarget::resolve(&subject, (410.0, 200.0), &slots, &[]).expect("target");
        let command = found.command(&subject, tab).expect("command");
        let mut moved = layout.clone();
        moved.apply(command).expect("moves c out beside b");
        assert_eq!(moved.showing("").expect("tab").group_count(), 3, "a, c, b");
    }
}
