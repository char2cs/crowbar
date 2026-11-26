use protocol::PaneId;

use crate::error::{Error, Result};
use crate::layout::command::Command;
use crate::layout::tab::Tab;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::{Layout, MAX_TABS};
use crate::tab_id::TabId;
use crate::tree::placement::Placement;

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
    layout.show(tab);
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
    layout.show(tab);
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
    let shown = tab.id();
    layout.show(shown);
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
    layout.show(to);
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
    layout.show(to);
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
    let mut tab = Tab::new(new_tab, title, source.space.clone());
    tab.receive(moved, None)?;
    tab.focus(pane)?;
    layout.tabs.insert(from + 1, tab);
    layout.show(new_tab);
    Ok(())
}

#[cfg(test)]
mod tests {
    use protocol::{PaneId, PaneState, Params};
    use serde_json::json;

    use crate::error::Error;
    use crate::error::Error::{
        DuplicatePane, DuplicateTab, NotAChat, OnlyPane, PaneNotFound, SamePane, SameTab,
        TabNotFound,
    };
    use crate::layout::Layout;
    use crate::layout::command::Command;
    use crate::layout::tab::{MAX_PANES_PER_TAB, Tab};
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{add, chat, instance, layout as build};
    use crate::test_support::tab_fixtures::{at, id};
    use crate::tree::edge::Edge;
    use crate::tree::placement::Placement;

    fn open(
        tab: u64,
        name: &str,
    ) -> Command {
        crate::test_support::layout_fixtures::open(tab, instance(name))
    }

    fn placed(
        tab: u64,
        instance: crate::PaneInstance,
        placement: Option<Placement>,
    ) -> Command {
        let tab = TabId(tab);
        Command::OpenPane {
            tab,
            instance,
            placement,
        }
    }

    fn open_beside(
        tab: u64,
        name: &str,
        target: &str,
    ) -> Command {
        placed(tab, instance(name), Some(at(target, Edge::Right)))
    }

    fn open_chat(
        tab: u64,
        name: &str,
        placement: Option<Placement>,
    ) -> Command {
        placed(tab, chat(name, "chat-7"), placement)
    }

    fn move_to_tab(
        pane: &str,
        to: u64,
        placement: Option<Placement>,
    ) -> Command {
        let (pane, to) = (id(pane), TabId(to));
        Command::MovePaneToTab {
            pane,
            to,
            placement,
        }
    }

    fn detach(
        pane: &str,
        new_tab: u64,
    ) -> Command {
        let (pane, new_tab) = (id(pane), TabId(new_tab));
        let title = "alone".to_owned();
        Command::DetachPane {
            pane,
            new_tab,
            title,
        }
    }

    fn close(pane: &str) -> Command {
        Command::ClosePane { pane: id(pane) }
    }

    fn active(layout: &Layout) -> Option<TabId> {
        layout.showing("").map(Tab::id)
    }

    fn focus_in(
        layout: &Layout,
        tab: u64,
    ) -> Option<&PaneId> {
        layout.tab(TabId(tab)).and_then(Tab::focused)
    }

    fn order(
        layout: &Layout,
        tab: u64,
    ) -> Vec<String> {
        let ids = layout.tab(TabId(tab)).map(Tab::pane_ids);
        let ids = ids.unwrap_or_default();
        ids.iter().map(ToString::to_string).collect()
    }

    fn tab_ids(layout: &Layout) -> Vec<u64> {
        layout.tabs().iter().map(|tab| tab.id().0).collect()
    }

    /// Tab 1 holds `a`, `b` and tab 2 holds `c`.
    fn two_tabs() -> Layout {
        build(vec![
            add(1),
            open(1, "a"),
            open(1, "b"),
            add(2),
            open(2, "c"),
        ])
    }

    /// Tab 1 holds chat `a` with editor tab `t1` and chat `b`; tab 2 holds chat `c`.
    fn two_chats_one_with_a_tab() -> Layout {
        let chat_b = placed(1, chat("b", "chat-b"), Some(at("a", Edge::Right)));
        build(vec![
            add(1),
            placed(1, chat("a", "chat-a"), None),
            open(1, "t1"),
            chat_b,
            add(2),
            placed(2, chat("c", "chat-c"), None),
        ])
    }

    /// Tab 1 holds chat `p1` (chat-7) alone, tab 2 holds `t2`.
    fn chat_and_a_tab() -> Layout {
        build(vec![
            add(1),
            open_chat(1, "p1", None),
            add(2),
            open(2, "t2"),
        ])
    }

    fn refused(
        fixture: fn() -> Layout,
        cases: Vec<(Command, Error)>,
    ) {
        for (command, error) in cases {
            let mut layout = fixture();
            let before = layout.clone();
            assert_eq!(layout.apply(command), Err(error));
            assert_eq!(layout, before, "unchanged");
        }
    }

    fn refused_in_two_tabs() -> Vec<(Command, Error)> {
        let swap = |a: &str, b: &str| Command::SwapPanes { a: id(a), b: id(b) };
        let moving = |pane: &str, target: &str| Command::MovePane {
            pane: id(pane),
            placement: at(target, Edge::Left),
        };
        let (pane, state, params) = (id("q"), PaneState::default(), Params::default());
        let ghost = Some(at("ghost", Edge::Left));
        vec![
            (open(2, "a"), DuplicatePane(id("a"))),
            (open(9, "n"), TabNotFound(TabId(9))),
            (close("z"), PaneNotFound(id("z"))),
            (Command::FocusPane { pane: id("z") }, PaneNotFound(id("z"))),
            (moving("b", "b"), SamePane(id("b"))),
            (moving("z", "a"), PaneNotFound(id("z"))),
            (moving("a", "c"), PaneNotFound(id("c"))),
            (move_to_tab("a", 1, None), SameTab(TabId(1))),
            (move_to_tab("a", 9, None), TabNotFound(TabId(9))),
            (move_to_tab("z", 2, None), PaneNotFound(id("z"))),
            (move_to_tab("a", 2, ghost), PaneNotFound(id("ghost"))),
            (swap("a", "c"), PaneNotFound(id("c"))),
            (swap("a", "a"), SamePane(id("a"))),
            (
                Command::SetPaneState {
                    pane: pane.clone(),
                    state,
                },
                PaneNotFound(id("q")),
            ),
            (
                Command::SetPaneParams { pane, params },
                PaneNotFound(id("q")),
            ),
        ]
    }

    #[test]
    fn refused_commands_say_why_and_change_nothing() {
        refused(two_tabs, refused_in_two_tabs());
        let with_chats = vec![
            (detach("c", 7), OnlyPane(id("c"))),
            (detach("t1", 7), NotAChat(id("t1"))),
            (detach("a", 2), DuplicateTab(TabId(2))),
            (detach("z", 7), PaneNotFound(id("z"))),
        ];
        refused(two_chats_one_with_a_tab, with_chats);
        let reveal = open_chat(2, "x", Some(at("ghost", Edge::Left)));
        refused(chat_and_a_tab, vec![(reveal, PaneNotFound(id("ghost")))]);
    }

    #[test]
    fn opening_a_pane_activates_its_tab_and_focuses_it() {
        let shown = Command::ActivateTab { tab: TabId(1) };
        let layout = build(vec![add(1), add(2), shown, open(2, "a")]);
        assert_eq!(active(&layout), Some(TabId(2)), "active");
        let home = layout.tab_of(&id("a")).map(Tab::id);
        assert_eq!(home, Some(TabId(2)), "home");
        assert_eq!(focus_in(&layout, 2), Some(&id("a")), "focus");
        let apart = build(vec![add(1), open(1, "t1"), open(1, "t2")]);
        assert_eq!(apart.pane_count(), 2, "no parameters, no identity");
    }

    #[test]
    fn a_chat_that_is_already_open_is_moved_not_duplicated() {
        let mut layout = chat_and_a_tab();
        layout.apply(open(1, "t1")).unwrap();
        let dropped = open_chat(2, "fresh", Some(at("t2", Edge::Left)));
        layout.apply(dropped).unwrap();
        assert_eq!(layout.pane_count(), 3, "no new pane");
        assert!(layout.pane(&id("fresh")).is_none(), "not added");
        assert_eq!(tab_ids(&layout), [2], "its emptied tab closed");
        let landed = order(&layout, 2);
        assert_eq!(landed, ["p1", "t1", "t2"], "dropped, with its tab");
        assert_eq!(active(&layout), Some(TabId(2)), "shown");
        assert_eq!(focus_in(&layout, 2), Some(&id("p1")), "focused");
        let mut layout = chat_and_a_tab();
        layout.apply(open_chat(2, "x", None)).unwrap();
        assert_eq!(tab_ids(&layout), [2], "emptied source closed");
        assert_eq!(order(&layout, 2), ["t2", "p1"], "right of the focus");
    }

    #[test]
    fn an_open_chat_dropped_in_its_own_tab_is_focused_or_rearranged() {
        let commands = vec![
            add(1),
            open_chat(1, "p1", None),
            open_beside(1, "t1", "p1"),
            open_beside(1, "t2", "t1"),
        ];
        let mut layout = build(commands.clone());
        let on_itself = open_chat(1, "x", Some(at("p1", Edge::Right)));
        layout.apply(on_itself).unwrap();
        assert_eq!(order(&layout, 1), ["p1", "t1", "t2"], "untouched");
        assert_eq!(focus_in(&layout, 1), Some(&id("p1")), "focused");
        let mut layout = build(commands);
        let beside = open_chat(1, "x", Some(at("t2", Edge::Right)));
        layout.apply(beside).unwrap();
        assert_eq!(order(&layout, 1), ["t1", "t2", "p1"], "moved");
        assert_eq!(layout.pane_count(), 3, "still three");
    }

    #[test]
    fn closing_a_pane_focuses_the_neighbour_that_takes_its_space() {
        let mut layout = build(vec![
            add(1),
            open(1, "a"),
            open_beside(1, "x", "a"),
            open_beside(1, "b", "a"),
            Command::FocusPane { pane: id("x") },
        ]);
        assert_eq!(order(&layout, 1), ["a", "b", "x"], "[[a|b]|x]");
        layout.apply(close("x")).unwrap();
        assert_eq!(focus_in(&layout, 1), Some(&id("b")), "adjacent pane");
    }

    #[test]
    fn closing_the_last_chat_of_a_tab_closes_the_editor_tabs_it_leaves_and_the_tab() {
        let mut layout = two_chats_one_with_a_tab();
        layout.apply(close("b")).unwrap();
        assert_eq!(order(&layout, 1), ["a", "t1"], "another chat keeps it");
        let removed = layout.execute(close("a")).unwrap();
        let names: Vec<_> = removed.iter().map(|p| p.pane_id.to_string()).collect();
        assert_eq!(names, ["a", "t1"], "the chat and the tab it left");
        assert_eq!(tab_ids(&layout), [2], "the chatless tab is gone");
    }

    #[test]
    fn closing_the_last_pane_of_a_tab_closes_the_tab_and_shows_the_adjacent_one() {
        let mut layout = build(vec![
            add(1),
            open(1, "a"),
            add(2),
            open(2, "b"),
            add(3),
            open(3, "c"),
            Command::ActivateTab { tab: TabId(2) },
        ]);
        let removed = layout.execute(close("b")).unwrap();
        assert_eq!(removed.len(), 1, "the instance is handed back");
        assert_eq!(tab_ids(&layout), [1, 3], "tab 2 is gone");
        assert_eq!(active(&layout), Some(TabId(3)), "the next tab");
        layout.apply(close("c")).unwrap();
        assert_eq!(active(&layout), Some(TabId(1)), "then the previous");
        layout.apply(close("a")).unwrap();
        assert!(layout.tabs().is_empty(), "no tabs");
        assert!(active(&layout).is_none(), "empty state");
        let mut layout = two_tabs();
        layout.apply(close("c")).unwrap();
        layout.apply(open(1, "c")).unwrap();
        layout.apply(add(2)).unwrap();
        layout.apply(close("a")).unwrap();
        assert_eq!(active(&layout), Some(TabId(2)), "background close");
    }

    #[test]
    fn focusing_moving_and_swapping_inside_a_tab() {
        let mut layout = two_tabs();
        layout.apply(open(1, "d")).unwrap();
        layout.apply(Command::FocusPane { pane: id("a") }).unwrap();
        assert_eq!(active(&layout), Some(TabId(1)), "focus shows the tab");
        assert_eq!(focus_in(&layout, 1), Some(&id("a")), "focused");
        let placement = at("a", Edge::Left);
        let moving = Command::MovePane {
            pane: id("d"),
            placement,
        };
        layout.apply(moving).unwrap();
        assert_eq!(order(&layout, 1), ["d", "a", "b"], "moved");
        let swap = Command::SwapPanes {
            a: id("a"),
            b: id("b"),
        };
        layout.apply(swap).unwrap();
        assert_eq!(order(&layout, 1), ["d", "b", "a"], "swapped");
    }

    #[test]
    fn moving_a_pane_to_another_tab_keeps_its_identity_and_state() {
        let mut layout = two_tabs();
        let state = PaneState::new(json!({"scroll": 9})).unwrap();
        let (pane, kept) = (id("a"), state.clone());
        let set = Command::SetPaneState { pane, state: kept };
        layout.apply(set).unwrap();
        layout
            .apply(move_to_tab("a", 2, Some(at("c", Edge::Top))))
            .unwrap();
        assert_eq!(order(&layout, 1), ["b"], "left");
        assert_eq!(order(&layout, 2), ["a", "c"], "arrived on the edge");
        let moved = layout.pane(&id("a")).map(|p| &p.state);
        assert_eq!(moved, Some(&state), "state kept");
        assert_eq!(active(&layout), Some(TabId(2)), "destination shown");
        assert_eq!(focus_in(&layout, 2), Some(&id("a")), "focused");
        layout.apply(move_to_tab("b", 2, None)).unwrap();
        assert_eq!(tab_ids(&layout), [2], "no empty tab left behind");
        let after = order(&layout, 2);
        assert_eq!(after, ["a", "b", "c"], "right of the focused pane");
    }

    #[test]
    fn a_full_destination_refuses_a_move_and_the_pane_stays() {
        let mut commands = vec![add(1), open(1, "a"), open(1, "a2"), add(2)];
        commands.extend((0..MAX_PANES_PER_TAB).map(|n| open(2, &format!("f{n}"))));
        let mut layout = build(commands);
        let before = layout.clone();
        let result = layout.apply(move_to_tab("a", 2, None));
        assert!(
            matches!(result, Err(Error::TooManyPanes { .. })),
            "{result:?}"
        );
        assert_eq!(layout, before, "unchanged");
    }

    #[test]
    fn a_chat_moves_or_detaches_with_its_editor_tabs() {
        let mut layout = two_chats_one_with_a_tab();
        layout.apply(move_to_tab("a", 2, None)).unwrap();
        assert_eq!(order(&layout, 1), ["b"], "left behind");
        assert_eq!(order(&layout, 2), ["c", "a", "t1"], "the whole group");
        assert_eq!(focus_in(&layout, 2), Some(&id("a")), "moved chat focused");
        let mut layout = two_chats_one_with_a_tab();
        layout
            .apply(Command::ActivateTab { tab: TabId(2) })
            .unwrap();
        layout.apply(detach("a", 7)).unwrap();
        layout.validate().unwrap();
        assert_eq!(tab_ids(&layout), [1, 7, 2], "right of the source tab");
        assert_eq!(order(&layout, 7), ["a", "t1"], "with its tab");
        assert_eq!(order(&layout, 1), ["b"], "siblings stay");
        let title = layout.tab(TabId(7)).map(Tab::title);
        assert_eq!(title, Some("alone"), "title");
        assert_eq!(active(&layout), Some(TabId(7)), "shown from background");
        assert_eq!(focus_in(&layout, 1), Some(&id("b")), "source focus");
    }

    #[test]
    fn state_and_params_go_to_the_pane_wherever_it_is() {
        let mut layout = two_tabs();
        let state = PaneState::new(json!({"draft": "hello"})).unwrap();
        let (pane, kept) = (id("c"), state.clone());
        layout
            .apply(Command::SetPaneState { pane, state: kept })
            .unwrap();
        let stored = layout.pane(&id("c")).map(|p| &p.state);
        assert_eq!(stored, Some(&state), "stored");
        let params = Params::default().with("threadId", "made").unwrap();
        let (pane, keyed) = (id("b"), params.clone());
        let rekey = Command::SetPaneParams {
            pane,
            params: keyed,
        };
        layout.apply(rekey).unwrap();
        let rekeyed = layout.pane(&id("b")).map(|p| &p.params);
        assert_eq!(rekeyed, Some(&params), "re-keyed");
        assert_eq!(order(&layout, 1), ["a", "b"], "in place");
        assert_eq!(active(&layout), Some(TabId(2)), "focus unchanged");
    }
}
