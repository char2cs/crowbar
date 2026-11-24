use protocol::PaneId;

use crate::command::Command;
use crate::error::{Error, Result};
use crate::layout::{Layout, MAX_TABS};
use crate::pane_instance::PaneInstance;
use crate::placement::Placement;
use crate::tab::Tab;
use crate::tab_id::TabId;

pub(crate) fn open(
    layout: &mut Layout,
    tab: TabId,
    instance: PaneInstance,
    placement: Option<Placement>,
) -> Result<()> {
    if layout.pane(&instance.pane_id).is_some() {
        return Err(Error::DuplicatePane(instance.pane_id));
    }
    layout.index_of(tab)?;
    if let Some(open_already) = layout.find_pane(&instance.type_id, &instance.params) {
        let existing = open_already.pane_id.clone();
        return reveal(layout, &existing, tab, placement);
    }
    layout.tab_mut(tab)?.open(instance, placement)?;
    layout.active = Some(tab);
    Ok(())
}

fn reveal(
    layout: &mut Layout,
    existing: &PaneId,
    tab: TabId,
    placement: Option<Placement>,
) -> Result<()> {
    let home = layout.index_of_pane(existing)?;
    let home_id = layout.tabs.get(home).map(Tab::id);
    if home_id != Some(tab) {
        move_to_tab(layout, existing, tab, placement)?;
        return focus(layout, existing);
    }
    let target = layout.tab_mut(tab)?;
    if let Some(placement) = placement.filter(|placement| &placement.target != existing) {
        target.relocate(existing, &placement)?;
    }
    target.focus(existing)?;
    layout.active = Some(tab);
    Ok(())
}

pub(crate) fn close(
    layout: &mut Layout,
    pane: &PaneId,
) -> Result<Vec<PaneInstance>> {
    let index = layout.index_of_pane(pane)?;
    let tab = layout
        .tabs
        .get_mut(index)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    let instance = tab.take_pane(pane)?;
    let chatless = instance.type_id.is_chat() && tab.chat_count() == 0;
    let mut closed = vec![instance];
    if chatless {
        closed.extend(left_behind(tab)?);
    }
    if tab.is_empty() {
        layout.take_tab_at(index);
    }
    Ok(closed)
}

fn left_behind(tab: &mut Tab) -> Result<Vec<PaneInstance>> {
    tab.pane_ids()
        .iter()
        .map(|pane| tab.take_pane(pane))
        .collect()
}

pub(crate) fn focus(
    layout: &mut Layout,
    pane: &PaneId,
) -> Result<()> {
    let tab = layout.tab_of_mut(pane)?;
    tab.focus(pane)?;
    layout.active = Some(tab.id());
    Ok(())
}

pub(crate) fn relocate(
    layout: &mut Layout,
    pane: &PaneId,
    placement: &Placement,
) -> Result<()> {
    layout.tab_of_mut(pane)?.relocate(pane, placement)
}

pub(crate) fn swap(
    layout: &mut Layout,
    a: &PaneId,
    b: &PaneId,
) -> Result<()> {
    layout.tab_of_mut(a)?.swap(a, b)
}

pub(crate) fn set_content(
    layout: &mut Layout,
    command: Command,
) -> Result<()> {
    match command {
        Command::SetPaneState { pane, state } => layout.tab_of_mut(&pane)?.set_state(&pane, state),
        Command::SetPaneParams { pane, params } => {
            layout.tab_of_mut(&pane)?.set_params(&pane, params)
        },
        _ => Ok(()),
    }
}

pub(crate) fn move_to_tab(
    layout: &mut Layout,
    pane: &PaneId,
    to: TabId,
    placement: Option<Placement>,
) -> Result<()> {
    let from = layout.index_of_pane(pane)?;
    let destination = layout.index_of(to)?;
    if from == destination {
        return Err(Error::SameTab(to));
    }
    if layout.tabs.get(from).is_some_and(|tab| tab.is_chat(pane)) {
        return move_group(layout, pane, (from, to), placement);
    }
    layout.tab_mut(to)?.check_open(pane, placement.as_ref())?;
    let source = layout
        .tabs
        .get_mut(from)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    let instance = source.take_pane(pane)?;
    let emptied = source.is_empty();
    layout.tab_mut(to)?.open(instance, placement)?;
    if emptied {
        layout.take_tab_at(from);
    }
    layout.active = Some(to);
    Ok(())
}

fn move_group(
    layout: &mut Layout,
    pane: &PaneId,
    route: (usize, TabId),
    placement: Option<Placement>,
) -> Result<()> {
    let (from, to) = route;
    let members = layout
        .tabs
        .get(from)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?
        .group_members(pane)?;
    layout
        .tab_mut(to)?
        .check_receive(&members, placement.as_ref())?;
    let source = layout
        .tabs
        .get_mut(from)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    let moved = source.take_group(pane)?;
    let emptied = source.is_empty();
    let destination = layout.tab_mut(to)?;
    destination.receive(moved, placement)?;
    destination.focus(pane)?;
    if emptied {
        layout.take_tab_at(from);
    }
    layout.active = Some(to);
    Ok(())
}

pub(crate) fn detach(
    layout: &mut Layout,
    pane: &PaneId,
    new_tab: TabId,
    title: String,
) -> Result<()> {
    let from = layout.index_of_pane(pane)?;
    if layout.tab(new_tab).is_some() {
        return Err(Error::DuplicateTab(new_tab));
    }
    if layout.tabs.len() >= MAX_TABS {
        return Err(Error::TooManyTabs { limit: MAX_TABS });
    }
    let source = layout
        .tabs
        .get_mut(from)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    if !source.is_chat(pane) {
        return Err(Error::NotAChat(pane.clone()));
    }
    if source.chat_count() < 2 {
        return Err(Error::OnlyPane(pane.clone()));
    }
    let moved = source.take_group(pane)?;
    let mut tab = Tab::new(new_tab, title);
    tab.receive(moved, None)?;
    tab.focus(pane)?;
    layout.tabs.insert(from + 1, tab);
    layout.active = Some(new_tab);
    Ok(())
}

#[cfg(test)]
mod tests {
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

    fn chat(
        name: &str,
        chat_id: &str,
    ) -> PaneInstance {
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

    fn open(
        tab: u64,
        name: &str,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance: instance(name),
            placement: None,
        }
    }

    fn open_beside(
        tab: u64,
        name: &str,
        target: &str,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance: instance(name),
            placement: Some(at(target, Edge::Right)),
        }
    }

    fn open_chat(
        tab: u64,
        name: &str,
        chat_id: &str,
        placement: Option<Placement>,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance: chat(name, chat_id),
            placement,
        }
    }

    fn at(
        target: &str,
        edge: Edge,
    ) -> Placement {
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

    fn order(
        layout: &Layout,
        tab: u64,
    ) -> Vec<String> {
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
        assert_eq!(tab_ids(&layout), [2], "its emptied tab closed");
        assert_eq!(
            order(&layout, 2),
            ["p1", "t1", "t2"],
            "landed where it was dropped, with its editor tab"
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
            open_beside(1, "t1", "p1"),
            open_beside(1, "t2", "t1"),
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
    fn closing_the_last_chat_of_a_tab_closes_the_editor_tabs_it_leaves_and_the_tab() {
        let mut layout = two_chats_one_with_a_tab();
        layout.apply(Command::ClosePane { pane: id("b") }).unwrap();
        assert_eq!(order(&layout, 1), ["a", "t1"], "another chat keeps the tab");
        let removed = layout
            .execute(Command::ClosePane { pane: id("a") })
            .unwrap();
        let names: Vec<_> = removed.iter().map(|p| p.pane_id.to_string()).collect();
        assert_eq!(names, ["a", "t1"], "the chat and the tab it left");
        assert_eq!(tab_ids(&layout), [2], "the chatless tab is gone");
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

    fn move_to_tab(
        pane: &str,
        to: u64,
        placement: Option<Placement>,
    ) -> Command {
        Command::MovePaneToTab {
            pane: id(pane),
            to: TabId(to),
            placement,
        }
    }

    #[test]
    fn moving_to_another_tab_validates_everything_before_changing_anything() {
        let mut layout = layout(vec![add(1), open(1, "a"), add(2), open(2, "c")]);
        let before = layout.clone();
        let cases = [
            (move_to_tab("a", 1, None), Error::SameTab(TabId(1))),
            (move_to_tab("a", 9, None), Error::TabNotFound(TabId(9))),
            (move_to_tab("z", 2, None), Error::PaneNotFound(id("z"))),
            (
                move_to_tab("a", 2, Some(at("ghost", Edge::Left))),
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

    fn two_chats_one_with_a_tab() -> Layout {
        layout(vec![
            add(1),
            open_chat(1, "a", "chat-a", None),
            open(1, "t1"),
            open_chat(1, "b", "chat-b", Some(at("a", Edge::Right))),
            add(2),
            open_chat(2, "c", "chat-c", None),
        ])
    }

    #[test]
    fn moving_a_chat_to_another_tab_takes_its_editor_tabs_with_it() {
        let mut layout = two_chats_one_with_a_tab();
        layout.apply(move_to_tab("a", 2, None)).unwrap();
        assert_eq!(order(&layout, 1), ["b"], "left behind");
        assert_eq!(order(&layout, 2), ["c", "a", "t1"], "the whole group");
        assert_eq!(
            layout.tab(TabId(2)).and_then(|t| t.focused()),
            Some(&id("a")),
            "the moved chat is focused"
        );
        layout.validate().unwrap();
    }

    #[test]
    fn detaching_moves_the_chat_and_its_tabs_to_a_new_tab_right_of_its_own_and_shows_it() {
        let mut layout = two_chats_one_with_a_tab();
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
        assert_eq!(order(&layout, 7), ["a", "t1"], "with its tab");
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
        layout.validate().unwrap();
    }

    fn detach(
        pane: &str,
        new_tab: u64,
    ) -> Command {
        Command::DetachPane {
            pane: id(pane),
            new_tab: TabId(new_tab),
            title: String::new(),
        }
    }

    #[test]
    fn detaching_refuses_what_it_cannot_do_and_changes_nothing() {
        let mut layout = two_chats_one_with_a_tab();
        let before = layout.clone();
        let cases = [
            (detach("c", 7), Error::OnlyPane(id("c"))),
            (detach("t1", 7), Error::NotAChat(id("t1"))),
            (detach("a", 2), Error::DuplicateTab(TabId(2))),
            (detach("z", 7), Error::PaneNotFound(id("z"))),
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

    #[test]
    fn re_keying_a_pane_keeps_its_place_and_focus() {
        let mut layout = layout(vec![
            add(1),
            open(1, "a"),
            open(1, "b"),
            add(2),
            open(2, "c"),
        ]);
        let params = Params::default().with("threadId", "made").unwrap();
        layout
            .apply(Command::SetPaneParams {
                pane: id("b"),
                params: params.clone(),
            })
            .unwrap();
        assert_eq!(
            layout.pane(&id("b")).map(|p| &p.params),
            Some(&params),
            "re-keyed"
        );
        assert_eq!(order(&layout, 1), ["a", "b"], "in place");
        assert_eq!(active(&layout), Some(TabId(2)), "focus unchanged");
        assert_eq!(
            layout.apply(Command::SetPaneParams {
                pane: id("q"),
                params
            }),
            Err(Error::PaneNotFound(id("q"))),
            "unknown"
        );
    }
}
