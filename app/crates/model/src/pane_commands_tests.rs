use protocol::{PaneId, PaneState, PaneTypeId, Params};
use serde_json::json;

use crate::command::Command;
use crate::edge::Edge;
use crate::error::Error;
use crate::layout::Layout;
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::tab::{MAX_PANES_PER_TAB, Tab};
use crate::tab_id::TabId;

fn id(name: &str) -> PaneId {
    PaneId::new(name).unwrap()
}

fn instance(name: &str) -> PaneInstance {
    PaneInstance {
        pane_id: id(name),
        type_id: PaneTypeId::new("core.terminal").unwrap(),
        params: Params::default(),
        state: PaneState::default(),
    }
}

fn chat(name: &str, chat_id: &str) -> PaneInstance {
    PaneInstance {
        type_id: PaneTypeId::new("core.chat").unwrap(),
        params: Params::default().with("chatId", chat_id).unwrap(),
        ..instance(name)
    }
}

fn add(tab: u64) -> Command {
    Command::AddTab {
        id: TabId(tab),
        title: format!("tab {tab}"),
    }
}

fn open(tab: u64, name: &str) -> Command {
    Command::OpenPane {
        tab: TabId(tab),
        instance: instance(name),
        placement: None,
    }
}

fn open_chat(tab: u64, name: &str, chat_id: &str, placement: Option<Placement>) -> Command {
    Command::OpenPane {
        tab: TabId(tab),
        instance: chat(name, chat_id),
        placement,
    }
}

fn at(target: &str, edge: Edge) -> Placement {
    Placement {
        target: id(target),
        edge,
    }
}

fn layout(commands: Vec<Command>) -> Layout {
    let mut layout = Layout::default();
    for command in commands {
        layout.apply(command).unwrap();
    }
    layout.validate().unwrap();
    layout
}

fn active(layout: &Layout) -> Option<TabId> {
    layout.active_tab().map(Tab::id)
}

fn order(layout: &Layout, tab: u64) -> Vec<String> {
    layout.tab(TabId(tab)).map_or_else(Vec::new, |tab| {
        tab.pane_ids().iter().map(ToString::to_string).collect()
    })
}

fn tab_ids(layout: &Layout) -> Vec<u64> {
    layout.tabs().iter().map(|tab| tab.id().0).collect()
}

#[test]
fn pane_ids_are_unique_across_tabs_and_the_tab_must_exist() {
    let mut layout = layout(vec![add(1), add(2), open(1, "a")]);
    assert_eq!(
        layout.apply(open(2, "a")),
        Err(Error::DuplicatePane(id("a"))),
        "duplicate"
    );
    assert_eq!(
        layout.apply(open(9, "b")),
        Err(Error::TabNotFound(TabId(9))),
        "no tab"
    );
}

#[test]
fn opening_a_pane_activates_its_tab_and_focuses_it() {
    let layout = layout(vec![
        add(1),
        add(2),
        Command::ActivateTab { tab: TabId(1) },
        open(2, "a"),
    ]);
    assert_eq!(active(&layout), Some(TabId(2)), "active");
    assert_eq!(layout.tab_of(&id("a")).map(Tab::id), Some(TabId(2)), "home");
    assert_eq!(
        layout.tab(TabId(2)).and_then(|t| t.focused()),
        Some(&id("a")),
        "focus"
    );
}

#[test]
fn a_chat_that_is_already_open_is_moved_not_duplicated() {
    let mut layout = layout(vec![
        add(1),
        open_chat(1, "p1", "chat-7", None),
        open(1, "t1"),
        add(2),
        open(2, "t2"),
    ]);
    layout
        .apply(open_chat(2, "fresh", "chat-7", Some(at("t2", Edge::Left))))
        .unwrap();
    assert_eq!(layout.pane_count(), 3, "no new pane");
    assert_eq!(order(&layout, 1), ["t1"], "left its old tab");
    assert_eq!(
        order(&layout, 2),
        ["p1", "t2"],
        "landed where it was dropped"
    );
    assert!(
        layout.pane(&id("fresh")).is_none(),
        "the new instance was not added"
    );
    assert_eq!(active(&layout), Some(TabId(2)), "shown");
    assert_eq!(
        layout.tab(TabId(2)).and_then(|t| t.focused()),
        Some(&id("p1")),
        "focused"
    );
}

#[test]
fn opening_a_chat_whose_tab_it_would_empty_closes_that_tab() {
    let mut layout = layout(vec![
        add(1),
        open_chat(1, "p1", "chat-7", None),
        add(2),
        open(2, "t2"),
    ]);
    layout.apply(open_chat(2, "x", "chat-7", None)).unwrap();
    assert_eq!(tab_ids(&layout), [2], "emptied source closed");
    assert_eq!(order(&layout, 2), ["t2", "p1"], "right of the focused pane");
    layout.validate().unwrap();
}

#[test]
fn dropping_a_chat_on_its_own_pane_only_focuses_it() {
    let mut layout = layout(vec![
        add(1),
        open_chat(1, "p1", "chat-7", None),
        open(1, "t1"),
    ]);
    let before_order = order(&layout, 1);
    layout
        .apply(open_chat(1, "x", "chat-7", Some(at("p1", Edge::Right))))
        .unwrap();
    assert_eq!(order(&layout, 1), before_order, "arrangement untouched");
    assert_eq!(
        layout.tab(TabId(1)).and_then(|t| t.focused()),
        Some(&id("p1")),
        "focus"
    );
}

#[test]
fn an_open_chat_dropped_beside_another_pane_of_its_tab_is_rearranged() {
    let mut layout = layout(vec![
        add(1),
        open_chat(1, "p1", "chat-7", None),
        open(1, "t1"),
        open(1, "t2"),
    ]);
    layout
        .apply(open_chat(1, "x", "chat-7", Some(at("t2", Edge::Right))))
        .unwrap();
    assert_eq!(order(&layout, 1), ["t1", "t2", "p1"], "moved");
    assert_eq!(layout.pane_count(), 3, "still three");
}

#[test]
fn panes_without_parameters_are_not_identified_with_each_other() {
    let layout = layout(vec![add(1), open(1, "t1"), open(1, "t2")]);
    assert_eq!(layout.pane_count(), 2, "two terminals");
}

#[test]
fn a_failed_reveal_changes_nothing() {
    let mut layout = layout(vec![
        add(1),
        open_chat(1, "p1", "chat-7", None),
        add(2),
        open(2, "t2"),
    ]);
    let before = layout.clone();
    let result = layout.apply(open_chat(2, "x", "chat-7", Some(at("ghost", Edge::Left))));
    assert_eq!(result, Err(Error::PaneNotFound(id("ghost"))), "bad target");
    assert_eq!(layout, before, "unchanged");
}

#[test]
fn closing_a_pane_focuses_the_neighbour_that_takes_its_space() {
    let mut layout = layout(vec![
        add(1),
        open(1, "a"),
        Command::OpenPane {
            tab: TabId(1),
            instance: instance("x"),
            placement: Some(at("a", Edge::Right)),
        },
        Command::OpenPane {
            tab: TabId(1),
            instance: instance("b"),
            placement: Some(at("a", Edge::Right)),
        },
        Command::FocusPane { pane: id("x") },
    ]);
    assert_eq!(order(&layout, 1), ["a", "b", "x"], "[[a|b]|x]");
    layout.apply(Command::ClosePane { pane: id("x") }).unwrap();
    assert_eq!(
        layout.tab(TabId(1)).and_then(|t| t.focused()),
        Some(&id("b")),
        "adjacent pane"
    );
}

#[test]
fn closing_the_last_pane_of_a_tab_closes_the_tab_and_shows_the_adjacent_one() {
    let mut layout = layout(vec![
        add(1),
        open(1, "a"),
        add(2),
        open(2, "b"),
        add(3),
        open(3, "c"),
        Command::ActivateTab { tab: TabId(2) },
    ]);
    let removed = layout
        .execute(Command::ClosePane { pane: id("b") })
        .unwrap();
    assert_eq!(removed.len(), 1, "the instance is handed back");
    assert_eq!(tab_ids(&layout), [1, 3], "tab 2 is gone");
    assert_eq!(active(&layout), Some(TabId(3)), "the next tab is shown");
    layout.apply(Command::ClosePane { pane: id("c") }).unwrap();
    assert_eq!(active(&layout), Some(TabId(1)), "then the previous one");
    layout.apply(Command::ClosePane { pane: id("a") }).unwrap();
    assert!(
        layout.tabs().is_empty() && active(&layout).is_none(),
        "empty state"
    );
    assert_eq!(
        layout.apply(Command::ClosePane { pane: id("a") }),
        Err(Error::PaneNotFound(id("a"))),
        "again"
    );
}

#[test]
fn closing_a_pane_in_a_background_tab_keeps_the_active_tab() {
    let mut layout = layout(vec![add(1), open(1, "a"), add(2), open(2, "b")]);
    layout.apply(Command::ClosePane { pane: id("a") }).unwrap();
    assert_eq!(active(&layout), Some(TabId(2)), "active kept");
    assert_eq!(tab_ids(&layout), [2], "emptied background tab closed");
}

#[test]
fn focusing_a_pane_activates_its_tab() {
    let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b"), add(2)]);
    layout.apply(Command::FocusPane { pane: id("a") }).unwrap();
    assert_eq!(active(&layout), Some(TabId(1)), "tab");
    assert_eq!(
        layout.tab(TabId(1)).and_then(|t| t.focused()),
        Some(&id("a")),
        "pane"
    );
    assert_eq!(
        layout.apply(Command::FocusPane { pane: id("z") }),
        Err(Error::PaneNotFound(id("z"))),
        "missing"
    );
}

#[test]
fn moving_a_pane_within_its_tab_rearranges_it() {
    let mut layout = layout(vec![add(1), open(1, "a"), open(1, "b"), open(1, "c")]);
    layout
        .apply(Command::MovePane {
            pane: id("c"),
            placement: at("a", Edge::Left),
        })
        .unwrap();
    assert_eq!(order(&layout, 1), ["c", "a", "b"], "moved");
    assert_eq!(
        layout.apply(Command::MovePane {
            pane: id("c"),
            placement: at("c", Edge::Left)
        }),
        Err(Error::SamePane(id("c"))),
        "self"
    );
}

#[test]
fn a_pane_cannot_be_moved_next_to_a_pane_of_another_tab() {
    let mut layout = layout(vec![
        add(1),
        open(1, "a"),
        open(1, "b"),
        add(2),
        open(2, "c"),
    ]);
    let before = layout.clone();
    assert_eq!(
        layout.apply(Command::MovePane {
            pane: id("a"),
            placement: at("c", Edge::Left)
        }),
        Err(Error::PaneNotFound(id("c"))),
        "other tab"
    );
    assert_eq!(layout, before, "unchanged");
}

#[test]
fn moving_a_pane_to_another_tab_keeps_its_identity_and_state() {
    let mut layout = layout(vec![
        add(1),
        open(1, "a"),
        open(1, "b"),
        add(2),
        open(2, "c"),
    ]);
    let state = PaneState::new(json!({"scroll": 9})).unwrap();
    layout
        .apply(Command::SetPaneState {
            pane: id("a"),
            state: state.clone(),
        })
        .unwrap();
    layout
        .apply(Command::MovePaneToTab {
            pane: id("a"),
            to: TabId(2),
            placement: Some(at("c", Edge::Top)),
        })
        .unwrap();
    assert_eq!(order(&layout, 1), ["b"], "left");
    assert_eq!(order(&layout, 2), ["a", "c"], "arrived on the edge");
    assert_eq!(
        layout.pane(&id("a")).map(|p| &p.state),
        Some(&state),
        "state kept"
    );
    assert_eq!(active(&layout), Some(TabId(2)), "destination shown");
    assert_eq!(
        layout.tab(TabId(2)).and_then(|t| t.focused()),
        Some(&id("a")),
        "focused"
    );
}

#[test]
fn moving_the_last_pane_of_a_tab_closes_that_tab() {
    let mut layout = layout(vec![add(1), open(1, "a"), add(2), open(2, "c")]);
    layout
        .apply(Command::MovePaneToTab {
            pane: id("a"),
            to: TabId(2),
            placement: None,
        })
        .unwrap();
    assert_eq!(tab_ids(&layout), [2], "no empty tab left behind");
    assert_eq!(order(&layout, 2), ["c", "a"], "right of the focused pane");
}

#[test]
fn moving_to_another_tab_validates_everything_before_changing_anything() {
    let mut layout = layout(vec![add(1), open(1, "a"), add(2), open(2, "c")]);
    let before = layout.clone();
    let cases = [
        (
            Command::MovePaneToTab {
                pane: id("a"),
                to: TabId(1),
                placement: None,
            },
            Error::SameTab(TabId(1)),
        ),
        (
            Command::MovePaneToTab {
                pane: id("a"),
                to: TabId(9),
                placement: None,
            },
            Error::TabNotFound(TabId(9)),
        ),
        (
            Command::MovePaneToTab {
                pane: id("z"),
                to: TabId(2),
                placement: None,
            },
            Error::PaneNotFound(id("z")),
        ),
        (
            Command::MovePaneToTab {
                pane: id("a"),
                to: TabId(2),
                placement: Some(at("ghost", Edge::Left)),
            },
            Error::PaneNotFound(id("ghost")),
        ),
    ];
    for (command, error) in cases {
        assert_eq!(layout.apply(command), Err(error));
        assert_eq!(layout, before, "unchanged");
    }
}

#[test]
fn a_full_destination_refuses_a_move_and_the_pane_stays() {
    let mut commands = vec![add(1), open(1, "a"), open(1, "a2"), add(2)];
    commands.extend((0..MAX_PANES_PER_TAB).map(|n| open(2, &format!("f{n}"))));
    let mut layout = layout(commands);
    let before = layout.clone();
    let result = layout.apply(Command::MovePaneToTab {
        pane: id("a"),
        to: TabId(2),
        placement: None,
    });
    assert!(
        matches!(result, Err(Error::TooManyPanes { .. })),
        "{result:?}"
    );
    assert_eq!(layout, before, "unchanged");
}

#[test]
fn detaching_moves_the_pane_to_a_new_tab_right_of_its_own_and_shows_it() {
    let mut layout = layout(vec![
        add(1),
        open(1, "a"),
        open(1, "b"),
        add(2),
        open(2, "c"),
    ]);
    layout.apply(Command::FocusPane { pane: id("a") }).unwrap();
    layout
        .apply(Command::ActivateTab { tab: TabId(2) })
        .unwrap();
    layout
        .apply(Command::DetachPane {
            pane: id("a"),
            new_tab: TabId(7),
            title: "alone".into(),
        })
        .unwrap();
    assert_eq!(tab_ids(&layout), [1, 7, 2], "right of the source tab");
    assert_eq!(order(&layout, 7), ["a"], "alone");
    assert_eq!(order(&layout, 1), ["b"], "siblings stay");
    assert_eq!(layout.tab(TabId(7)).map(Tab::title), Some("alone"), "title");
    assert_eq!(
        active(&layout),
        Some(TabId(7)),
        "shown even when the source was in the background"
    );
    assert_eq!(
        layout.tab(TabId(1)).and_then(|t| t.focused()),
        Some(&id("b")),
        "source focus repaired"
    );
}

#[test]
fn detaching_refuses_what_it_cannot_do_and_changes_nothing() {
    let mut layout = layout(vec![
        add(1),
        open(1, "a"),
        open(1, "b"),
        add(2),
        open(2, "c"),
    ]);
    let before = layout.clone();
    let cases = [
        (
            Command::DetachPane {
                pane: id("c"),
                new_tab: TabId(7),
                title: String::new(),
            },
            Error::OnlyPane(id("c")),
        ),
        (
            Command::DetachPane {
                pane: id("a"),
                new_tab: TabId(2),
                title: String::new(),
            },
            Error::DuplicateTab(TabId(2)),
        ),
        (
            Command::DetachPane {
                pane: id("z"),
                new_tab: TabId(7),
                title: String::new(),
            },
            Error::PaneNotFound(id("z")),
        ),
    ];
    for (command, error) in cases {
        assert_eq!(layout.apply(command), Err(error));
        assert_eq!(layout, before, "unchanged");
    }
}

#[test]
fn swapping_needs_two_panes_of_one_tab() {
    let mut layout = layout(vec![
        add(1),
        open(1, "a"),
        open(1, "b"),
        add(2),
        open(2, "c"),
    ]);
    layout
        .apply(Command::SwapPanes {
            a: id("a"),
            b: id("b"),
        })
        .unwrap();
    assert_eq!(order(&layout, 1), ["b", "a"], "swapped");
    assert_eq!(
        layout.apply(Command::SwapPanes {
            a: id("a"),
            b: id("c")
        }),
        Err(Error::PaneNotFound(id("c"))),
        "other tab"
    );
    assert_eq!(
        layout.apply(Command::SwapPanes {
            a: id("a"),
            b: id("a")
        }),
        Err(Error::SamePane(id("a"))),
        "self"
    );
}

#[test]
fn state_goes_to_the_pane_wherever_it_is() {
    let mut layout = layout(vec![add(1), open(1, "a"), add(2), open(2, "c")]);
    let state = PaneState::new(json!({"draft": "hello"})).unwrap();
    layout
        .apply(Command::SetPaneState {
            pane: id("c"),
            state: state.clone(),
        })
        .unwrap();
    assert_eq!(
        layout.pane(&id("c")).map(|p| &p.state),
        Some(&state),
        "stored"
    );
    assert_eq!(
        layout.apply(Command::SetPaneState {
            pane: id("q"),
            state
        }),
        Err(Error::PaneNotFound(id("q"))),
        "unknown"
    );
}
