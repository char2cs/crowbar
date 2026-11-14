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

fn view(id: u64, panes: &[&str]) -> TabView {
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

const AT: &[(f32, Option<TabDrop>)] = &[
    (5.0, Some(TabDrop::Reorder(0))),
    (50.0, Some(TabDrop::Merge(TabId(1)))),
    (95.0, Some(TabDrop::Reorder(1))),
    (105.0, Some(TabDrop::Reorder(1))),
    (150.0, Some(TabDrop::Merge(TabId(2)))),
    (295.0, Some(TabDrop::Reorder(3))),
    (900.0, Some(TabDrop::Reorder(3))),
];

#[test]
fn the_outer_quarters_of_a_slot_are_gaps_and_the_middle_is_the_tab() {
    for (x, expected) in AT {
        assert_eq!(TabDrop::at(*x, &spans(), &ids()), *expected, "x = {x}");
    }
}

#[test]
fn an_empty_strip_has_no_drop() {
    assert_eq!(TabDrop::at(10.0, &[], &[]), None, "nothing to drop on");
}

#[test]
fn a_pane_dropped_on_a_tab_joins_it() {
    let commands =
        TabDrop::Merge(TabId(1)).commands(&DragSubject::Pane(pane("c")), fresh(), &tabs());
    assert_eq!(
        commands,
        vec![Command::MovePaneToTab {
            pane: pane("c"),
            to: TabId(1),
            placement: None
        }]
    );
}

#[test]
fn a_chat_dropped_on_a_tab_opens_beside_the_focused_pane() {
    let commands =
        TabDrop::Merge(TabId(3)).commands(&DragSubject::Open(instance()), fresh(), &tabs());
    assert_eq!(
        commands,
        vec![Command::OpenPane {
            tab: TabId(3),
            instance: instance(),
            placement: None
        }]
    );
}

#[test]
fn a_chat_dropped_between_tabs_gets_a_tab_of_its_own_at_that_index() {
    let commands = TabDrop::Reorder(1).commands(&DragSubject::Open(instance()), fresh(), &tabs());
    assert_eq!(
        commands,
        vec![
            Command::AddTab {
                id: TabId(9),
                title: "New".into()
            },
            Command::OpenPane {
                tab: TabId(9),
                instance: instance(),
                placement: None
            },
            Command::MoveTab {
                tab: TabId(9),
                to_index: 1
            },
        ]
    );
}

#[test]
fn a_pane_of_a_split_dropped_between_tabs_detaches_and_moves() {
    let commands = TabDrop::Reorder(0).commands(&DragSubject::Pane(pane("b")), fresh(), &tabs());
    assert_eq!(
        commands,
        vec![
            Command::DetachPane {
                pane: pane("b"),
                new_tab: TabId(9),
                title: "New".into()
            },
            Command::MoveTab {
                tab: TabId(9),
                to_index: 0
            },
        ]
    );
}

#[test]
fn the_only_pane_of_a_tab_dropped_between_tabs_just_moves_its_tab() {
    let to_end = TabDrop::Reorder(3).commands(&DragSubject::Pane(pane("a")), fresh(), &tabs());
    assert_eq!(
        to_end,
        vec![Command::MoveTab {
            tab: TabId(1),
            to_index: 2
        }],
        "gap after the last tab"
    );
    let in_place = TabDrop::Reorder(0).commands(&DragSubject::Pane(pane("a")), fresh(), &tabs());
    assert_eq!(
        in_place,
        vec![Command::MoveTab {
            tab: TabId(1),
            to_index: 0
        }],
        "gap before itself"
    );
}

#[test]
fn a_tab_drag_yields_no_commands_here() {
    let commands = TabDrop::Reorder(0).commands(&DragSubject::Tab(TabId(2)), fresh(), &tabs());
    assert!(commands.is_empty(), "the strip handles its own tabs");
}
