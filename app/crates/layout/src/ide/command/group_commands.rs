use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::command::group_state;
use crate::layout::Layout;
use crate::layout::tab::pane_instance::PaneInstance;

type Seat<'a> = (&'a PaneId, Option<usize>);

pub(crate) fn open(
    layout: &mut Layout,
    instance: PaneInstance,
    onto: Seat<'_>,
) -> Result<()> {
    if layout.pane(&instance.pane_id).is_some() {
        return Err(Error::DuplicatePane(instance.pane_id));
    }
    if instance.type_id.is_chat() {
        return Err(Error::ChatIsNotATab(instance.pane_id));
    }
    let home = layout.index_of_pane(onto.0)?;
    if let Some(open_already) = layout.find_pane(&instance.type_id, &instance.params) {
        let existing = open_already.pane_id.clone();
        return join(layout, &existing, onto);
    }
    let tab = layout
        .tabs
        .get_mut(home)
        .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?;
    tab.open_in_group(instance, onto)?;
    layout.show_at(home);
    Ok(())
}

pub(crate) fn join(
    layout: &mut Layout,
    pane: &PaneId,
    onto: Seat<'_>,
) -> Result<()> {
    if layout.tab_of(pane).is_some_and(|tab| tab.is_chat(pane)) {
        return Err(Error::ChatIsNotATab(pane.clone()));
    }
    let pinned = layout
        .tab_of(pane)
        .and_then(|tab| tab.group_of(pane))
        .is_some_and(|group| group.is_pinned(pane));
    seat_in(layout, pane, onto)?;
    if pinned {
        group_state::pin(layout, pane)?;
    }
    Ok(())
}

fn seat_in(
    layout: &mut Layout,
    pane: &PaneId,
    onto: Seat<'_>,
) -> Result<()> {
    let from = layout.index_of_pane(pane)?;
    let destination = layout.index_of_pane(onto.0)?;
    if from == destination {
        let tab = layout
            .tabs
            .get_mut(from)
            .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
        tab.join_group(pane, onto)?;
        layout.show_at(from);
        return Ok(());
    }
    layout
        .tabs
        .get(destination)
        .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?
        .check_open_in_group(pane, onto)?;
    let source = layout
        .tabs
        .get_mut(from)
        .ok_or_else(|| Error::PaneNotFound(pane.clone()))?;
    let instance = source.take_pane(pane)?;
    let emptied = source.is_empty();
    let target = layout
        .tabs
        .get_mut(destination)
        .ok_or_else(|| Error::PaneNotFound(onto.0.clone()))?;
    target.open_in_group(instance, onto)?;
    let shown = target.id();
    if emptied {
        layout.take_tab_at(from);
    }
    layout.show(shown);
    Ok(())
}

#[cfg(test)]
mod tests {
    use protocol::{PaneTypeId, Params};
    use serde_json::json;

    use super::*;
    use crate::PaneGroup;
    use crate::error::Error::{DuplicatePane, IndexOutOfRange, PaneNotFound};
    use crate::layout::command::Command;
    use crate::layout::layout_history::LayoutHistory;
    use crate::layout::stored::layout_stored::{load, store};
    use crate::layout::tab::Tab;
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::add;
    use crate::test_support::tab_fixtures::{at, editor as pane, id, screen};
    use crate::tree::edge::Edge;

    type Name = &'static str;

    /// A pane, the member it lands beside, the index it asks for, and the strip it ends in.
    type Seat = (Name, Name, Option<usize>, Name);

    /// The pane focused first, the pane closed, the strip left and the pane focused after.
    type Closing = (Name, Name, Name, Name);

    /// A pane dropped on an edge of a member, and tab 1's strips after.
    type Dropped = (Name, Name, Edge, Name);

    const PLACED: &[Seat] = &[
        ("n", "a", None, "a b c n*"),
        ("n", "b", Some(0), "n* a b c"),
        ("n", "c", Some(2), "a b n* c"),
        ("n", "a", Some(3), "a b c n*"),
        ("n", "d", None, "d n*"),
    ];

    /// Opening a copy of an open pane moves that pane instead.
    const REOPENED: &[Seat] = &[
        ("a", "b", Some(2), "b c a*"),
        ("d", "a", None, "a b c d*"),
        ("z", "a", Some(1), "a z* b c"),
    ];

    /// Tab 1's strips after the move.
    const MOVED: &[Seat] = &[
        ("c", "a", Some(0), "c* a b | d* | e f*"),
        ("a", "c", None, "b c a* | d* | e f*"),
        ("a", "b", Some(1), "b a* c | d* | e f*"),
        ("b", "a", Some(1), "a b* c | d* | e f*"),
        ("b", "e", None, "a c* | d* | e f b*"),
        ("b", "f", Some(0), "a c* | d* | b* e f"),
        ("d", "a", Some(1), "a d* b c | e f*"),
        ("a", "d", None, "b c* | d a* | e f*"),
    ];

    const CLOSING: &[Closing] = &[
        ("a", "a", "b* c", "b"),
        ("b", "b", "a c*", "c"),
        ("c", "c", "a b*", "b"),
        ("c", "a", "b c*", "c"),
        ("a", "c", "a* b", "a"),
        ("d", "d", "e f*", "f"),
        ("c", "f", "e*", "c"),
        ("c", "e", "f*", "c"),
    ];

    const DROPPED: &[Dropped] = &[
        ("b", "a", Edge::Right, "a c* | b* | d* | e f*"),
        ("b", "c", Edge::Left, "b* | a c* | d* | e f*"),
        ("d", "a", Edge::Left, "d* | a b c* | e f*"),
        ("b", "e", Edge::Right, "a c* | d* | e f* | b*"),
        ("f", "d", Edge::Left, "a b c* | f* | d* | e*"),
    ];

    fn apply(
        layout: &mut Layout,
        command: Command,
    ) {
        layout.apply(command).unwrap();
        layout.validate().unwrap();
    }

    fn open_cmd(
        instance: PaneInstance,
        onto: &str,
        index: Option<usize>,
    ) -> Command {
        let onto = id(onto);
        Command::OpenPaneInGroup {
            instance,
            onto,
            index,
        }
    }

    fn join_cmd(
        moved: &str,
        onto: &str,
        index: Option<usize>,
    ) -> Command {
        let (pane, onto) = (id(moved), id(onto));
        Command::MoveToGroup { pane, onto, index }
    }

    fn split(
        layout: &mut Layout,
        tab: u64,
        new: &str,
    ) {
        let tab = TabId(tab);
        let focused = layout.tab(tab).and_then(Tab::focused);
        let placement = focused.map(|target| at(target.as_str(), Edge::Right));
        let instance = pane(new);
        let command = Command::OpenPane {
            tab,
            instance,
            placement,
        };
        apply(layout, command);
    }

    fn chat(name: &str) -> PaneInstance {
        PaneInstance {
            type_id: PaneTypeId::new(PaneTypeId::CHAT).unwrap(),
            params: Params::default().with("chatId", name).unwrap(),
            ..pane(name)
        }
    }

    fn focus(
        layout: &mut Layout,
        name: &str,
    ) {
        apply(layout, Command::FocusPane { pane: id(name) });
    }

    fn refuses(
        command: Command,
        error: &Error,
    ) {
        let mut layout = fixture();
        let before = layout.clone();
        let result = layout.apply(command);
        assert_eq!(result.as_ref(), Err(error));
        assert_eq!(layout, before, "{error}: unchanged");
    }

    /// Tab 1 reads `[a b c*] [d*] [e f*]` left to right, `c` focused, tab 2 holds `[x y*]` and
    /// tab 3 holds `[z]`.
    fn fixture() -> Layout {
        let mut layout = Layout::default();
        let groups = [(1, "a", "bc"), (1, "d", ""), (1, "e", "f")];
        let others = [(2, "x", "y"), (3, "z", "")];
        for (tab, first, stacked) in groups.into_iter().chain(others) {
            group(&mut layout, tab, first, stacked);
        }
        focus(&mut layout, "c");
        layout
    }

    fn group(
        layout: &mut Layout,
        tab: u64,
        first: &str,
        stacked: &str,
    ) {
        if layout.tab(TabId(tab)).is_none() {
            apply(layout, add(tab));
        }
        split(layout, tab, first);
        for name in stacked.chars().map(String::from) {
            apply(layout, open_cmd(pane(&name), first, None));
        }
    }

    fn marked(
        pane: &PaneId,
        shown: &PaneId,
    ) -> String {
        if pane == shown {
            return format!("{pane}*");
        }
        pane.to_string()
    }

    fn written(group: &PaneGroup) -> Option<String> {
        let shown = group.shown()?;
        let names: Vec<String> = group.tabs().iter().map(|p| marked(p, shown)).collect();
        Some(names.join(" "))
    }

    /// Each group of a tab as its members in order, the shown one starred.
    fn strips(
        layout: &Layout,
        tab: u64,
    ) -> Vec<String> {
        let tab = layout.tab(TabId(tab)).unwrap();
        let leaves = tab.leaves(screen(3000.0, 1000.0));
        leaves.iter().filter_map(|(_, g)| written(g)).collect()
    }

    fn strip_with(
        layout: &Layout,
        tab: u64,
        name: &str,
    ) -> String {
        let all = strips(layout, tab);
        let found = all.into_iter().find(|strip| strip.contains(name));
        found.unwrap_or_default()
    }

    fn focused(layout: &Layout) -> String {
        let tab = layout.showing("");
        let focused = tab.and_then(|tab| tab.focused().map(ToString::to_string));
        focused.unwrap_or_default()
    }

    fn shown_tab(layout: &Layout) -> Option<TabId> {
        layout.showing("").map(Tab::id)
    }

    fn pinned(
        layout: &Layout,
        name: &str,
    ) -> bool {
        let pane = id(name);
        let group = layout.tab_of(&pane).and_then(|tab| tab.group_of(&pane));
        group.is_some_and(|group| group.is_pinned(&pane))
    }

    #[test]
    fn a_chat_never_becomes_an_editor_tab() {
        let refused = Err(Error::ChatIsNotATab(id("q")));
        let mut layout = fixture();
        let opening = open_cmd(chat("q"), "a", None);
        assert_eq!(layout.apply(opening), refused, "opened");
        let placement = Some(at("z", Edge::Right));
        let beside = Command::OpenPane {
            tab: TabId(3),
            instance: chat("q"),
            placement,
        };
        apply(&mut layout, beside);
        let before = layout.clone();
        assert_eq!(layout.apply(join_cmd("q", "a", None)), refused, "moved");
        assert_eq!(layout, before, "unchanged");
    }

    #[test]
    fn groups_are_counted_and_found_by_any_member() {
        let layout = fixture();
        let tab = layout.tab(TabId(1)).unwrap();
        assert_eq!((tab.group_count(), tab.len()), (3, 6), "places, panes");
        let stacked = tab.group_of(&id("b")).unwrap();
        assert_eq!(stacked.tabs(), [id("a"), id("b"), id("c")], "order");
        assert_eq!(stacked.active_tab(), Some(&id("c")), "shown");
        assert!(tab.group_of(&id("x")).is_none(), "another tab's pane");
        assert_eq!(tab.pane_ids().len(), 6, "hidden panes are listed");
        let slots = tab.layout(screen(3000.0, 1000.0));
        assert_eq!(slots.len(), 3, "one slot per group");
    }

    #[test]
    fn a_pane_opens_at_the_requested_place_of_a_strip_and_is_shown() {
        for &(new, onto, index, strip) in PLACED {
            let mut layout = fixture();
            apply(&mut layout, open_cmd(pane(new), onto, index));
            assert_eq!(strip_with(&layout, 1, new), strip, "{onto} {index:?}");
            assert_eq!(focused(&layout), new, "{onto} {index:?}: focused");
        }
        let mut layout = fixture();
        apply(&mut layout, open_cmd(pane("n"), "z", None));
        assert_eq!(strips(&layout, 3), ["z n*"], "another tab's group");
        assert_eq!(shown_tab(&layout), Some(TabId(3)), "that tab is shown");
    }

    #[test]
    fn a_pinned_pane_stays_pinned_in_the_group_it_moves_to() {
        for (onto, tab, strip) in [("e", 1, "a* e f"), ("x", 2, "a* x y")] {
            let mut layout = fixture();
            apply(&mut layout, Command::PinPane { pane: id("a") });
            apply(&mut layout, join_cmd("a", onto, None));
            let group = layout.tab_of(&id("a")).and_then(|t| t.group_of(&id("a")));
            let pins = group.map(PaneGroup::pinned_count);
            let seated = (strip_with(&layout, tab, "a"), pins);
            assert_eq!(seated, (strip.to_owned(), Some(1)), "{onto}");
            assert!(pinned(&layout, "a"), "{onto}: still pinned");
        }
    }

    #[test]
    fn a_pane_that_is_open_somewhere_is_moved_not_opened_twice() {
        for &(existing, onto, index, strip) in REOPENED {
            let mut layout = fixture();
            let copy = PaneInstance {
                pane_id: id("copy"),
                ..pane(existing)
            };
            apply(&mut layout, open_cmd(copy, onto, index));
            assert_eq!(layout.pane_count(), 9, "{existing}: nothing new");
            assert!(layout.pane(&id("copy")).is_none(), "{existing}: dropped");
            assert_eq!(strip_with(&layout, 1, existing), strip, "{existing}");
            assert_eq!(focused(&layout), existing, "{existing}: focus");
        }
    }

    #[test]
    fn opening_or_moving_into_a_group_is_refused_without_changes() {
        let range = |index, len| IndexOutOfRange { index, len };
        let opening = [
            ("n", "zz", None, PaneNotFound(id("zz"))),
            ("n", "a", Some(4), range(4, 3)),
            ("b", "a", None, DuplicatePane(id("b"))),
            ("x", "a", None, DuplicatePane(id("x"))),
            ("n", "d", Some(2), range(2, 1)),
        ];
        for (new, onto, index, error) in opening {
            let params = Params::default().with("terminalId", "fresh").unwrap();
            let instance = PaneInstance {
                params,
                ..pane(new)
            };
            refuses(open_cmd(instance, onto, index), &error);
        }
        let moving = [
            ("zz", "a", None, PaneNotFound(id("zz"))),
            ("a", "zz", None, PaneNotFound(id("zz"))),
            ("a", "b", Some(3), range(3, 2)),
            ("a", "d", Some(2), range(2, 1)),
        ];
        for (moved, onto, index, error) in moving {
            refuses(join_cmd(moved, onto, index), &error);
        }
    }

    #[test]
    fn a_tab_stops_at_its_pane_limit_inside_groups_too() {
        let mut layout = Layout::default();
        apply(&mut layout, add(1));
        split(&mut layout, 1, "p0");
        for index in 1..crate::layout::tab::MAX_PANES_PER_TAB {
            let instance = pane(&format!("p{index}"));
            apply(&mut layout, open_cmd(instance, "p0", None));
        }
        let before = layout.clone();
        let more = open_cmd(pane("more"), "p0", None);
        let error = layout.apply(more).unwrap_err();
        assert!(matches!(error, Error::TooManyPanes { .. }), "{error:?}");
        assert_eq!(layout, before, "unchanged");
    }

    #[test]
    fn closing_a_member_shows_its_neighbour_and_hands_over_focus() {
        for &(shown, closed, strip, after) in CLOSING {
            let mut layout = fixture();
            focus(&mut layout, shown);
            apply(&mut layout, Command::ClosePane { pane: id(closed) });
            let all = strips(&layout, 1);
            let listed = all.join(" | ");
            let case = format!("{shown} then close {closed}: {listed}");
            assert!(all.iter().any(|group| group == strip), "{case}");
            assert_eq!(focused(&layout), after, "{case}");
        }
        let mut layout = fixture();
        focus(&mut layout, "d");
        apply(&mut layout, Command::ClosePane { pane: id("d") });
        assert_eq!(strips(&layout, 1), ["a b c*", "e f*"], "space given");
        focus(&mut layout, "z");
        apply(&mut layout, Command::ClosePane { pane: id("z") });
        assert!(layout.tab(TabId(3)).is_none(), "emptied tab gone");
        assert_eq!(shown_tab(&layout), Some(TabId(2)), "next tab");
    }

    #[test]
    fn focusing_a_hidden_member_shows_it_and_leaves_other_groups_alone() {
        let mut layout = fixture();
        focus(&mut layout, "a");
        assert_eq!(strips(&layout, 1), ["a* b c", "d*", "e f*"], "strips");
        focus(&mut layout, "y");
        assert_eq!(strips(&layout, 2), ["x y*"], "other tab");
        assert_eq!(shown_tab(&layout), Some(TabId(2)), "its tab");
    }

    #[test]
    fn a_pane_moves_between_strips_and_reorders_inside_one() {
        for &(moved, onto, index, expected) in MOVED {
            let mut layout = fixture();
            apply(&mut layout, join_cmd(moved, onto, index));
            let case = format!("{moved} onto {onto} at {index:?}");
            assert_eq!(strips(&layout, 1).join(" | "), expected, "{case}");
            assert_eq!(focused(&layout), moved, "{case}: focus");
        }
    }

    #[test]
    fn moving_across_tabs_closes_only_an_emptied_source_and_shows_the_destination() {
        let mut layout = fixture();
        apply(&mut layout, join_cmd("z", "a", Some(0)));
        assert!(layout.tab(TabId(3)).is_none(), "emptied tab closed");
        let joined = strips(&layout, 1);
        assert_eq!(joined, ["z* a b c", "d*", "e f*"], "joined");
        assert_eq!(shown_tab(&layout), Some(TabId(1)), "destination shown");
        let mut layout = fixture();
        apply(&mut layout, join_cmd("x", "z", None));
        assert_eq!(strips(&layout, 2), ["y*"], "source keeps its rest");
        assert_eq!(strips(&layout, 3), ["z x*"], "destination");
        assert_eq!(shown_tab(&layout), Some(TabId(3)), "destination shown");
        assert_eq!(focused(&layout), "x", "focus");
    }

    #[test]
    fn dropping_a_tab_on_an_edge_takes_it_out_of_its_strip() {
        let drop = |pane: &str, target: &str, edge| Command::MovePane {
            pane: id(pane),
            placement: at(target, edge),
        };
        for &(moved, target, edge, expected) in DROPPED {
            let mut layout = fixture();
            apply(&mut layout, drop(moved, target, edge));
            let case = format!("{moved} on {target} {edge:?}");
            assert_eq!(strips(&layout, 1).join(" | "), expected, "{case}");
        }
        let own_edge = drop("d", "d", Edge::Left);
        refuses(own_edge, &Error::SamePane(id("d")));
    }

    #[test]
    fn swapping_exchanges_places_and_pins_follow_the_panes() {
        let swap = |a: &str, b: &str| Command::SwapPanes { a: id(a), b: id(b) };
        let mut layout = fixture();
        apply(&mut layout, swap("c", "f"));
        let across = strips(&layout, 1);
        assert_eq!(across, ["a b f*", "d*", "e c*"], "across groups");
        assert_eq!(focused(&layout), "c", "focus follows the pane");
        let mut layout = fixture();
        apply(&mut layout, swap("a", "c"));
        let inside = strips(&layout, 1);
        assert_eq!(inside, ["c* b a", "d*", "e f*"], "inside a strip");
        let mut layout = fixture();
        apply(&mut layout, Command::PinPane { pane: id("a") });
        apply(&mut layout, swap("a", "f"));
        let seen = ["a", "b", "c", "e", "f"].map(|name| pinned(&layout, name));
        assert_eq!(seen, [true, false, false, false, false], "only a");
    }

    #[test]
    fn an_editor_tab_moves_to_another_tab_but_never_detaches_alone() {
        let mut layout = fixture();
        let before = layout.clone();
        let detach = Command::DetachPane {
            pane: id("b"),
            new_tab: TabId(9),
            title: "b".into(),
        };
        assert_eq!(layout.apply(detach), Err(Error::NotAChat(id("b"))));
        assert_eq!(layout, before, "unchanged");
        let (pane, to) = (id("b"), TabId(3));
        let moving = Command::MovePaneToTab {
            pane,
            to,
            placement: None,
        };
        apply(&mut layout, moving);
        assert_eq!(strips(&layout, 1), ["a c*", "d*", "e f*"], "source");
        assert_eq!(strips(&layout, 3), ["z b*"], "joins the focused group");
    }

    #[test]
    fn every_strip_command_is_one_undo_step_and_a_no_op_is_none() {
        let opening = open_cmd(pane("n"), "a", Some(1));
        let (pane, placement) = (id("b"), at("d", Edge::Bottom));
        let undoable = [
            opening,
            join_cmd("b", "e", None),
            join_cmd("a", "c", None),
            Command::ClosePane { pane: id("b") },
            Command::MovePane { pane, placement },
        ];
        for command in undoable {
            let mut history = LayoutHistory::new(fixture());
            let before = history.layout().clone();
            history.apply(command.clone()).unwrap();
            let after = history.layout().clone();
            assert_ne!(after, before, "{command:?}: changed");
            assert!(history.undo(), "{command:?}: undo");
            assert_eq!(history.layout(), &before, "{command:?}: undone");
            assert!(history.redo(), "{command:?}: redo");
            assert_eq!(history.layout(), &after, "{command:?}: redone");
        }
        let mut history = LayoutHistory::new(fixture());
        history.apply(join_cmd("c", "a", Some(2))).unwrap();
        assert!(!history.undo(), "a move that changes nothing");
    }

    fn stored_group(
        panes: &serde_json::Value,
        active: &str,
        focused: &str,
    ) -> String {
        let terminal = |name: &str| json!({"paneId": name, "type": "core.terminal", "params": {"id": name}, "state": null});
        let tree = json!({"node": "leaf", "chat": null, "tabs": panes, "active": active,
            "pinned": 0, "editorOpen": true, "chatSelected": false});
        let tab = json!({"id": 1, "title": "t", "space": "", "tree": tree,
            "focused": focused, "panes": {"a": terminal("a"), "b": terminal("b")}});
        json!({"version": crate::layout::stored::schema::SCHEMA_VERSION, "shown": {"": 1}, "tabs": [tab]}).to_string()
    }

    #[test]
    fn stacked_layouts_round_trip_and_a_broken_stored_group_is_refused() {
        let layout = fixture();
        let stored = store(&layout).unwrap();
        assert_eq!(load(&stored).unwrap(), layout, "round trip");
        let cases = [
            ("fine", json!(["a", "b"]), "a", "a", true),
            ("empty group", json!([]), "a", "a", false),
            ("shows a stranger", json!(["a", "b"]), "zz", "a", false),
            ("repeats a pane", json!(["a", "a", "b"]), "a", "a", false),
            ("misses a pane", json!(["a"]), "a", "a", false),
            ("focus on hidden", json!(["a", "b"]), "a", "b", false),
        ];
        for (name, panes, active, focused, loads) in cases {
            let loaded = load(stored_group(&panes, active, focused).as_bytes());
            assert_eq!(loaded.is_ok(), loads, "{name}: {loaded:?}");
        }
    }
}
