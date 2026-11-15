use model::{Command, DragSubject, TabId};

use crate::new_tab::NewTab;
use crate::span::Span;
use crate::tab_view::TabView;

const EDGE: f32 = 0.25;

/// Where a chat dragged from the sidebar or a pane handle would land on the strip.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TabDrop {
    /// Between tabs: a tab of its own at this index.
    Reorder(usize),
    /// On the middle of a tab: merged into that tab as a split.
    Merge(TabId),
}

impl TabDrop {
    /// The drop under pointer `x` among tab `spans` and their `tabs`: the outer quarter of a slot
    /// is the gap before or after it, the middle half is the tab itself.
    #[must_use]
    pub fn at(
        x: f32,
        spans: &[Span],
        tabs: &[TabId],
    ) -> Option<Self> {
        let last = spans.len().checked_sub(1)?;
        let index = spans.iter().position(|span| x < span.end()).unwrap_or(last);
        let span = spans.get(index)?;
        let across = (x - span.x) / span.width;
        if across < EDGE {
            return Some(Self::Reorder(index));
        }
        if across > 1.0 - EDGE {
            return Some(Self::Reorder(index + 1));
        }
        tabs.get(index).copied().map(Self::Merge)
    }

    /// The commands that carry out the drop of `subject`; `new_tab` names the tab a pane or chat
    /// gets when it does not merge. A tab drag is the strip's own reorder and yields nothing.
    #[must_use]
    pub fn commands(
        &self,
        subject: &DragSubject,
        new_tab: NewTab,
        tabs: &[TabView],
    ) -> Vec<Command> {
        match (subject, self) {
            (DragSubject::Pane(pane), Self::Merge(to)) => vec![move_into(pane, *to)],
            (DragSubject::Open(instance), Self::Merge(tab)) => vec![open_in(instance, *tab)],
            (DragSubject::Pane(pane), Self::Reorder(index)) => detach(pane, *index, new_tab, tabs),
            (DragSubject::Open(instance), Self::Reorder(index)) => {
                open_in_new_tab(instance, *index, new_tab, tabs.len())
            },
            (DragSubject::Tab(_), _) => Vec::new(),
        }
    }
}

fn move_into(
    pane: &protocol::PaneId,
    to: TabId,
) -> Command {
    Command::MovePaneToTab {
        pane: pane.clone(),
        to,
        placement: None,
    }
}

fn move_tab(
    tab: TabId,
    to_index: usize,
) -> Command {
    Command::MoveTab { tab, to_index }
}

fn open_in(
    instance: &model::PaneInstance,
    tab: TabId,
) -> Command {
    Command::OpenPane {
        tab,
        instance: instance.clone(),
        placement: None,
    }
}

fn open_in_new_tab(
    instance: &model::PaneInstance,
    index: usize,
    new_tab: NewTab,
    tab_count: usize,
) -> Vec<Command> {
    vec![
        Command::AddTab {
            id: new_tab.id,
            title: new_tab.title,
        },
        open_in(instance, new_tab.id),
        move_tab(new_tab.id, index.min(tab_count)),
    ]
}

fn detach(
    pane: &protocol::PaneId,
    gap: usize,
    new_tab: NewTab,
    tabs: &[TabView],
) -> Vec<Command> {
    let Some(position) = tabs
        .iter()
        .position(|tab| tab.members.iter().any(|member| &member.pane == pane))
    else {
        return Vec::new();
    };
    let Some(owner) = tabs.get(position) else {
        return Vec::new();
    };
    if owner.members.len() < 2 {
        let to_index = gap - usize::from(gap > position);
        return vec![move_tab(owner.id, to_index)];
    }
    vec![
        Command::DetachPane {
            pane: pane.clone(),
            new_tab: new_tab.id,
            title: new_tab.title,
        },
        move_tab(new_tab.id, gap.min(tabs.len())),
    ]
}

#[cfg(test)]
mod tests {
    use model::{Command, DragSubject, PaneInstance, TabId};
    use protocol::{PaneId, PaneState, PaneTypeId, Params};

    use super::*;
    use crate::member_view::MemberView;

    fn spans() -> Vec<Span> {
        (0..3u16)
            .map(|i| Span {
                x: f32::from(i) * 100.0,
                width: 100.0,
            })
            .collect()
    }

    fn ids() -> Vec<TabId> {
        vec![TabId(1), TabId(2), TabId(3)]
    }

    fn pane(name: &str) -> PaneId {
        PaneId::new(name).expect("id")
    }

    fn view(
        id: u64,
        panes: &[&str],
    ) -> TabView {
        let members = panes
            .iter()
            .map(|name| MemberView {
                pane: pane(name),
                label: (*name).to_owned().into(),
                provisional: false,
                working: false,
            })
            .collect();
        TabView {
            id: TabId(id),
            members,
        }
    }

    fn tabs() -> Vec<TabView> {
        vec![view(1, &["a"]), view(2, &["b", "c"]), view(3, &["d"])]
    }

    fn instance() -> PaneInstance {
        PaneInstance {
            pane_id: pane("new"),
            type_id: PaneTypeId::new("core.chat").expect("type"),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn fresh() -> NewTab {
        NewTab {
            id: TabId(9),
            title: "New".into(),
        }
    }

    struct Case {
        name: &'static str,
        drop: TabDrop,
        subject: fn() -> DragSubject,
        expected: fn() -> Vec<Command>,
    }

    const AT: &[(f32, Option<TabDrop>)] = &[
        (5.0, Some(TabDrop::Reorder(0))),
        (50.0, Some(TabDrop::Merge(TabId(1)))),
        (95.0, Some(TabDrop::Reorder(1))),
        (105.0, Some(TabDrop::Reorder(1))),
        (150.0, Some(TabDrop::Merge(TabId(2)))),
        (295.0, Some(TabDrop::Reorder(3))),
        (900.0, Some(TabDrop::Reorder(3))),
    ];

    const CASES: &[Case] = &[
        Case {
            name: "a pane dropped on a tab joins it",
            drop: TabDrop::Merge(TabId(1)),
            subject: || DragSubject::Pane(pane("c")),
            expected: || vec![move_into(&pane("c"), TabId(1))],
        },
        Case {
            name: "a chat dropped on a tab opens beside the focused pane",
            drop: TabDrop::Merge(TabId(3)),
            subject: || DragSubject::Open(instance()),
            expected: || vec![open_in(&instance(), TabId(3))],
        },
        Case {
            name: "a chat dropped between tabs gets a tab of its own at that index",
            drop: TabDrop::Reorder(1),
            subject: || DragSubject::Open(instance()),
            expected: || open_in_new_tab(&instance(), 1, fresh(), 3),
        },
        Case {
            name: "a pane of a split dropped between tabs detaches and moves",
            drop: TabDrop::Reorder(0),
            subject: || DragSubject::Pane(pane("b")),
            expected: || detached("b", 0),
        },
        Case {
            name: "the only pane dropped after the last tab moves its tab to the end",
            drop: TabDrop::Reorder(3),
            subject: || DragSubject::Pane(pane("a")),
            expected: || vec![move_tab(TabId(1), 2)],
        },
        Case {
            name: "the only pane dropped before itself keeps its place",
            drop: TabDrop::Reorder(0),
            subject: || DragSubject::Pane(pane("a")),
            expected: || vec![move_tab(TabId(1), 0)],
        },
        Case {
            name: "a tab drag yields no commands here",
            drop: TabDrop::Reorder(0),
            subject: || DragSubject::Tab(TabId(2)),
            expected: Vec::new,
        },
    ];

    fn detached(
        name: &str,
        to_index: usize,
    ) -> Vec<Command> {
        vec![
            Command::DetachPane {
                pane: pane(name),
                new_tab: TabId(9),
                title: "New".into(),
            },
            move_tab(TabId(9), to_index),
        ]
    }

    #[test]
    fn the_outer_quarters_of_a_slot_are_gaps_and_the_middle_is_the_tab() {
        for (x, expected) in AT {
            assert_eq!(TabDrop::at(*x, &spans(), &ids()), *expected, "x = {x}");
        }
        assert_eq!(TabDrop::at(10.0, &[], &[]), None, "an empty strip");
    }

    #[test]
    fn every_drop_yields_the_commands_that_carry_it_out() {
        for case in CASES {
            let commands = case.drop.commands(&(case.subject)(), fresh(), &tabs());
            assert_eq!(commands, (case.expected)(), "{}", case.name);
        }
    }
}
