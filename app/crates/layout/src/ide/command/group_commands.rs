use protocol::PaneId;

use crate::error::{Error, Result};
use crate::ide::command::pin_commands;
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
        pin_commands::pin(layout, pane)?;
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
    use crate::layout::tab::Tab;
    use protocol::{PaneState, PaneTypeId, Params};

    use super::*;
    use crate::layout::command::Command;
    use crate::layout::layout_history::LayoutHistory;
    use crate::tab_id::TabId;
    use crate::tree::edge::Edge;
    use crate::tree::geometry::rect::Rect;
    use crate::tree::placement::Placement;

    const SCREEN: Rect = Rect {
        x: 0.0,
        y: 0.0,
        width: 3000.0,
        height: 1000.0,
    };

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn pane(name: &str) -> PaneInstance {
        PaneInstance {
            pane_id: id(name),
            type_id: PaneTypeId::new("core.terminal").unwrap(),
            params: Params::default().with("terminalId", name).unwrap(),
            state: PaneState::default(),
        }
    }

    fn apply(
        layout: &mut Layout,
        command: Command,
    ) {
        layout.apply(command).unwrap();
        layout.validate().unwrap();
    }

    fn open_cmd(
        instance: PaneInstance,
        seat: (&str, Option<usize>),
    ) -> Command {
        Command::OpenPaneInGroup {
            instance,
            onto: id(seat.0),
            index: seat.1,
        }
    }

    fn join_cmd(
        moved: &str,
        seat: (&str, Option<usize>),
    ) -> Command {
        Command::MoveToGroup {
            pane: id(moved),
            onto: id(seat.0),
            index: seat.1,
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

    fn open_in(
        layout: &mut Layout,
        new: &str,
        seat: (&str, Option<usize>),
    ) {
        apply(layout, open_cmd(pane(new), seat));
    }

    fn stack(
        layout: &mut Layout,
        new: &str,
        onto: &str,
    ) {
        open_in(layout, new, (onto, None));
    }

    fn split(
        layout: &mut Layout,
        tab: u64,
        new: &str,
    ) {
        let placement = layout
            .tab(TabId(tab))
            .and_then(Tab::focused)
            .map(|target| Placement {
                target: target.clone(),
                edge: Edge::Right,
            });
        let command = Command::OpenPane {
            tab: TabId(tab),
            instance: pane(new),
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

    #[test]
    fn a_chat_never_becomes_an_editor_tab() {
        let opening = open_cmd(chat("q"), ("a", None));
        refuses(
            opening,
            &Error::ChatIsNotATab(id("q")),
            "opened onto a group",
        );
        let mut layout = fixture();
        let beside = Command::OpenPane {
            tab: TabId(3),
            instance: chat("q"),
            placement: Some(at("z", Edge::Right)),
        };
        apply(&mut layout, beside);
        let before = layout.clone();
        assert_eq!(
            layout.apply(join_cmd("q", ("a", None))),
            Err(Error::ChatIsNotATab(id("q"))),
            "moved onto a group"
        );
        assert_eq!(layout, before, "unchanged");
    }

    fn add_tab(
        layout: &mut Layout,
        tab: u64,
    ) {
        let command = Command::AddTab {
            id: TabId(tab),
            title: format!("tab {tab}"),
            space: crate::SpaceId::default(),
        };
        apply(layout, command);
    }

    fn join_to(
        layout: &mut Layout,
        moved: &str,
        seat: (&str, Option<usize>),
    ) {
        apply(layout, join_cmd(moved, seat));
    }

    fn focus(
        layout: &mut Layout,
        name: &str,
    ) {
        apply(layout, Command::FocusPane { pane: id(name) });
    }

    fn close(
        layout: &mut Layout,
        name: &str,
    ) {
        apply(layout, Command::ClosePane { pane: id(name) });
    }

    fn refuses(
        command: Command,
        error: &Error,
        name: &str,
    ) {
        let mut layout = fixture();
        let before = layout.clone();
        assert_eq!(layout.apply(command).as_ref(), Err(error), "{name}");
        assert_eq!(layout, before, "{name}: unchanged");
    }

    /// Tab 1 reads `[a b c] [d] [e f]` left to right, `c` focused after building, tab 2 holds
    /// `[x y]` and tab 3 holds `[z]`.
    fn fixture() -> Layout {
        let mut layout = Layout::default();
        add_tab(&mut layout, 1);
        split(&mut layout, 1, "a");
        stack(&mut layout, "b", "a");
        stack(&mut layout, "c", "a");
        split(&mut layout, 1, "d");
        split(&mut layout, 1, "e");
        stack(&mut layout, "f", "e");
        add_tab(&mut layout, 2);
        split(&mut layout, 2, "x");
        stack(&mut layout, "y", "x");
        add_tab(&mut layout, 3);
        split(&mut layout, 3, "z");
        focus(&mut layout, "c");
        layout
    }

    fn marked(
        name: &PaneId,
        active: &PaneId,
    ) -> String {
        if name == active {
            return format!("{name}*");
        }
        name.to_string()
    }

    fn written(
        names: &[PaneId],
        active: &PaneId,
    ) -> String {
        let marked: Vec<String> = names.iter().map(|name| marked(name, active)).collect();
        marked.join(" ")
    }

    fn strips(
        layout: &Layout,
        tab: u64,
    ) -> Vec<String> {
        let tab = layout.tab(TabId(tab)).unwrap();
        tab.leaves(SCREEN)
            .iter()
            .filter_map(|(_, group)| Some(written(group.tabs(), group.shown()?)))
            .collect()
    }

    fn focused(layout: &Layout) -> String {
        layout
            .showing("")
            .and_then(|tab| tab.focused().map(ToString::to_string))
            .unwrap_or_default()
    }

    fn shown_tab(layout: &Layout) -> Option<TabId> {
        layout.showing("").map(Tab::id)
    }

    #[test]
    fn groups_are_counted_and_found_by_any_member() {
        let layout = fixture();
        let tab = layout.tab(TabId(1)).unwrap();
        assert_eq!((tab.group_count(), tab.len()), (3, 6), "places and panes");
        let stacked = tab.group_of(&id("b")).unwrap();
        assert_eq!(stacked.tabs(), [id("a"), id("b"), id("c")], "strip order");
        assert_eq!(stacked.active_tab(), Some(&id("c")), "shown");
        assert!(tab.group_of(&id("x")).is_none(), "another tab's pane");
        assert_eq!(tab.pane_ids().len(), 6, "hidden panes are listed too");
        assert_eq!(tab.layout(SCREEN).len(), 3, "one slot per group");
    }

    struct Placed {
        name: &'static str,
        onto: &'static str,
        index: Option<usize>,
        strip: &'static str,
    }

    const PLACED: &[Placed] = &[
        Placed {
            name: "end",
            onto: "a",
            index: None,
            strip: "a b c n*",
        },
        Placed {
            name: "front",
            onto: "b",
            index: Some(0),
            strip: "n* a b c",
        },
        Placed {
            name: "middle",
            onto: "c",
            index: Some(2),
            strip: "a b n* c",
        },
        Placed {
            name: "explicit end",
            onto: "a",
            index: Some(3),
            strip: "a b c n*",
        },
        Placed {
            name: "onto a single",
            onto: "d",
            index: None,
            strip: "d n*",
        },
    ];

    #[test]
    fn a_pane_opens_at_the_requested_place_of_a_strip_and_is_shown() {
        for case in PLACED {
            let mut layout = fixture();
            open_in(&mut layout, "n", (case.onto, case.index));
            let all = strips(&layout, 1);
            let strip = all.iter().find(|strip| strip.contains('n')).unwrap();
            assert_eq!(strip, case.strip, "{}", case.name);
            assert_eq!(focused(&layout), "n", "{}: focused", case.name);
        }
    }

    struct Travelled {
        name: &'static str,
        moved: &'static str,
        onto: &'static str,
        tab: u64,
        strip: &'static str,
    }

    const TRAVELLED: &[Travelled] = &[
        Travelled {
            name: "into another group of the same tab",
            moved: "a",
            onto: "e",
            tab: 1,
            strip: "a* e f",
        },
        Travelled {
            name: "into a group of another tab",
            moved: "a",
            onto: "x",
            tab: 2,
            strip: "a* x y",
        },
    ];

    #[derive(Debug, PartialEq)]
    struct Seated {
        strip: String,
        pins: usize,
        moved_is_pinned: bool,
    }

    fn travelled(case: &Travelled) -> Seated {
        let mut layout = fixture();
        apply(
            &mut layout,
            Command::PinPane {
                pane: id(case.moved),
            },
        );
        join_to(&mut layout, case.moved, (case.onto, None));
        let strip = strips(&layout, case.tab)
            .into_iter()
            .find(|strip| strip.contains(case.moved))
            .unwrap_or_default();
        let moved = id(case.moved);
        let group = layout.tab_of(&moved).and_then(|tab| tab.group_of(&moved));
        let pinned = group.map(|group| (group.pinned_count(), group.is_pinned(&moved)));
        let (pins, moved_is_pinned) = pinned.unwrap_or_default();
        Seated {
            strip,
            pins,
            moved_is_pinned,
        }
    }

    fn still_pinned(case: &Travelled) -> Seated {
        Seated {
            strip: case.strip.to_owned(),
            pins: 1,
            moved_is_pinned: true,
        }
    }

    #[test]
    fn a_pinned_pane_stays_pinned_in_the_group_it_moves_to() {
        for case in TRAVELLED {
            assert_eq!(travelled(case), still_pinned(case), "{}", case.name);
        }
    }

    #[test]
    fn opening_into_a_group_of_another_tab_shows_that_tab() {
        let mut layout = fixture();
        stack(&mut layout, "n", "z");
        assert_eq!(strips(&layout, 3), ["z n*"], "joined");
        assert_eq!(shown_tab(&layout), Some(TabId(3)), "tab shown");
    }

    struct Reopened {
        name: &'static str,
        existing: &'static str,
        onto: &'static str,
        index: Option<usize>,
        strip: &'static str,
    }

    const REOPENED: &[Reopened] = &[
        Reopened {
            name: "from the same strip",
            existing: "a",
            onto: "b",
            index: Some(2),
            strip: "b c a*",
        },
        Reopened {
            name: "from another group",
            existing: "d",
            onto: "a",
            index: None,
            strip: "a b c d*",
        },
        Reopened {
            name: "from another tab",
            existing: "z",
            onto: "a",
            index: Some(1),
            strip: "a z* b c",
        },
    ];

    #[test]
    fn a_pane_that_is_open_somewhere_is_moved_not_opened_twice() {
        for case in REOPENED {
            let mut layout = fixture();
            let mut copy = pane(case.existing);
            copy.pane_id = id("copy");
            apply(&mut layout, open_cmd(copy, (case.onto, case.index)));
            assert_eq!(layout.pane_count(), 9, "{}: nothing new", case.name);
            assert!(layout.pane(&id("copy")).is_none(), "{}: dropped", case.name);
            let all = strips(&layout, 1);
            let strip = all.iter().find(|strip| strip.contains(case.existing));
            assert_eq!(strip.map(String::as_str), Some(case.strip), "{}", case.name);
            assert_eq!(focused(&layout), case.existing, "{}: focus", case.name);
        }
    }

    struct Refused {
        name: &'static str,
        new: &'static str,
        onto: &'static str,
        index: Option<usize>,
        error: fn() -> Error,
    }

    const REFUSED: &[Refused] = &[
        Refused {
            name: "unknown group",
            new: "n",
            onto: "zz",
            index: None,
            error: || Error::PaneNotFound(id("zz")),
        },
        Refused {
            name: "index past the end",
            new: "n",
            onto: "a",
            index: Some(4),
            error: || Error::IndexOutOfRange { index: 4, len: 3 },
        },
        Refused {
            name: "same pane id",
            new: "b",
            onto: "a",
            index: None,
            error: || Error::DuplicatePane(id("b")),
        },
        Refused {
            name: "same id in another tab",
            new: "x",
            onto: "a",
            index: None,
            error: || Error::DuplicatePane(id("x")),
        },
        Refused {
            name: "index past the end of a single",
            new: "n",
            onto: "d",
            index: Some(2),
            error: || Error::IndexOutOfRange { index: 2, len: 1 },
        },
    ];

    #[test]
    fn opening_into_a_group_is_refused_without_changes() {
        for case in REFUSED {
            let mut instance = pane(case.new);
            instance.params = Params::default().with("terminalId", "fresh").unwrap();
            let command = open_cmd(instance, (case.onto, case.index));
            refuses(command, &(case.error)(), case.name);
        }
    }

    #[test]
    fn a_tab_stops_at_its_pane_limit_inside_groups_too() {
        let mut layout = Layout::default();
        add_tab(&mut layout, 1);
        split(&mut layout, 1, "p0");
        for index in 1..crate::layout::tab::MAX_PANES_PER_TAB {
            stack(&mut layout, &format!("p{index}"), "p0");
        }
        let before = layout.clone();
        let command = Command::OpenPaneInGroup {
            instance: pane("more"),
            onto: id("p0"),
            index: None,
        };
        let error = layout.apply(command).unwrap_err();
        assert!(matches!(error, Error::TooManyPanes { .. }), "{error:?}");
        assert_eq!(layout, before, "unchanged");
    }

    struct Closing {
        name: &'static str,
        shown: &'static str,
        closed: &'static str,
        strip: &'static str,
        focus: &'static str,
    }

    const CLOSING: &[Closing] = &[
        Closing {
            name: "active first",
            shown: "a",
            closed: "a",
            strip: "b* c",
            focus: "b",
        },
        Closing {
            name: "active middle",
            shown: "b",
            closed: "b",
            strip: "a c*",
            focus: "c",
        },
        Closing {
            name: "active last",
            shown: "c",
            closed: "c",
            strip: "a b*",
            focus: "b",
        },
        Closing {
            name: "hidden member",
            shown: "c",
            closed: "a",
            strip: "b c*",
            focus: "c",
        },
        Closing {
            name: "hidden member while another is shown",
            shown: "a",
            closed: "c",
            strip: "a* b",
            focus: "a",
        },
        Closing {
            name: "only member of a group",
            shown: "d",
            closed: "d",
            strip: "e f*",
            focus: "f",
        },
        Closing {
            name: "shown in an unfocused group",
            shown: "c",
            closed: "f",
            strip: "e*",
            focus: "c",
        },
        Closing {
            name: "hidden in an unfocused group",
            shown: "c",
            closed: "e",
            strip: "f*",
            focus: "c",
        },
    ];

    #[test]
    fn closing_a_member_shows_its_neighbour_and_hands_over_focus() {
        for case in CLOSING {
            let mut layout = fixture();
            focus(&mut layout, case.shown);
            close(&mut layout, case.closed);
            let all = strips(&layout, 1);
            let listed = all.join(" | ");
            assert!(
                all.iter().any(|group| group == case.strip),
                "{}: {listed}",
                case.name
            );
            assert_eq!(focused(&layout), case.focus, "{}: {listed}", case.name);
        }
    }

    #[test]
    fn closing_the_only_pane_of_a_group_gives_its_space_to_the_neighbour() {
        let mut layout = fixture();
        focus(&mut layout, "d");
        close(&mut layout, "d");
        assert_eq!(strips(&layout, 1), ["a b c*", "e f*"], "strips");
    }

    #[test]
    fn closing_the_last_pane_of_a_tab_closes_the_tab_and_shows_the_next() {
        let mut layout = fixture();
        focus(&mut layout, "z");
        close(&mut layout, "z");
        assert!(layout.tab(TabId(3)).is_none(), "tab gone");
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

    struct Moved {
        name: &'static str,
        pane: &'static str,
        onto: &'static str,
        index: Option<usize>,
        strips: &'static [&'static str],
    }

    const MOVED: &[Moved] = &[
        Moved {
            name: "reorder to front",
            pane: "c",
            onto: "a",
            index: Some(0),
            strips: &["c* a b", "d*", "e f*"],
        },
        Moved {
            name: "reorder to end",
            pane: "a",
            onto: "c",
            index: None,
            strips: &["b c a*", "d*", "e f*"],
        },
        Moved {
            name: "reorder in the middle",
            pane: "a",
            onto: "b",
            index: Some(1),
            strips: &["b a* c", "d*", "e f*"],
        },
        Moved {
            name: "same place",
            pane: "b",
            onto: "a",
            index: Some(1),
            strips: &["a b* c", "d*", "e f*"],
        },
        Moved {
            name: "into another strip at the end",
            pane: "b",
            onto: "e",
            index: None,
            strips: &["a c*", "d*", "e f b*"],
        },
        Moved {
            name: "into another strip at an index",
            pane: "b",
            onto: "f",
            index: Some(0),
            strips: &["a c*", "d*", "b* e f"],
        },
        Moved {
            name: "a single joins a strip",
            pane: "d",
            onto: "a",
            index: Some(1),
            strips: &["a d* b c", "e f*"],
        },
        Moved {
            name: "a strip member onto a single",
            pane: "a",
            onto: "d",
            index: None,
            strips: &["b c*", "d a*", "e f*"],
        },
    ];

    #[test]
    fn a_pane_moves_between_strips_and_reorders_inside_one() {
        for case in MOVED {
            let mut layout = fixture();
            join_to(&mut layout, case.pane, (case.onto, case.index));
            assert_eq!(strips(&layout, 1), case.strips, "{}", case.name);
            assert_eq!(focused(&layout), case.pane, "{}: focus", case.name);
        }
    }

    #[test]
    fn moving_across_tabs_closes_an_emptied_source_and_shows_the_destination() {
        let mut layout = fixture();
        join_to(&mut layout, "z", ("a", Some(0)));
        assert!(layout.tab(TabId(3)).is_none(), "emptied tab closed");
        assert_eq!(strips(&layout, 1), ["z* a b c", "d*", "e f*"], "joined");
        assert_eq!(shown_tab(&layout), Some(TabId(1)), "destination shown");
    }

    #[test]
    fn moving_across_tabs_keeps_the_rest_of_the_source() {
        let mut layout = fixture();
        join_to(&mut layout, "x", ("z", None));
        assert_eq!(strips(&layout, 2), ["y*"], "source keeps its rest");
        assert_eq!(strips(&layout, 3), ["z x*"], "destination");
        assert_eq!(shown_tab(&layout), Some(TabId(3)), "destination shown");
        assert_eq!(focused(&layout), "x", "focus");
    }

    struct Impossible {
        name: &'static str,
        pane: &'static str,
        onto: &'static str,
        index: Option<usize>,
        error: fn() -> Error,
    }

    const IMPOSSIBLE: &[Impossible] = &[
        Impossible {
            name: "unknown pane",
            pane: "zz",
            onto: "a",
            index: None,
            error: || Error::PaneNotFound(id("zz")),
        },
        Impossible {
            name: "unknown group",
            pane: "a",
            onto: "zz",
            index: None,
            error: || Error::PaneNotFound(id("zz")),
        },
        Impossible {
            name: "index past the others in the same strip",
            pane: "a",
            onto: "b",
            index: Some(3),
            error: || Error::IndexOutOfRange { index: 3, len: 2 },
        },
        Impossible {
            name: "index past the end elsewhere",
            pane: "a",
            onto: "d",
            index: Some(2),
            error: || Error::IndexOutOfRange { index: 2, len: 1 },
        },
    ];

    #[test]
    fn moving_into_a_group_is_refused_without_changes() {
        for case in IMPOSSIBLE {
            let command = join_cmd(case.pane, (case.onto, case.index));
            refuses(command, &(case.error)(), case.name);
        }
    }

    struct Dropped {
        name: &'static str,
        pane: &'static str,
        target: &'static str,
        edge: Edge,
        strips: &'static [&'static str],
    }

    const DROPPED: &[Dropped] = &[
        Dropped {
            name: "beside its own strip",
            pane: "b",
            target: "a",
            edge: Edge::Right,
            strips: &["a c*", "b*", "d*", "e f*"],
        },
        Dropped {
            name: "left of its own strip",
            pane: "b",
            target: "c",
            edge: Edge::Left,
            strips: &["b*", "a c*", "d*", "e f*"],
        },
        Dropped {
            name: "a single moves",
            pane: "d",
            target: "a",
            edge: Edge::Left,
            strips: &["d*", "a b c*", "e f*"],
        },
        Dropped {
            name: "onto another strip",
            pane: "b",
            target: "e",
            edge: Edge::Right,
            strips: &["a c*", "d*", "e f*", "b*"],
        },
        Dropped {
            name: "a strip member beside a single",
            pane: "f",
            target: "d",
            edge: Edge::Left,
            strips: &["a b c*", "f*", "d*", "e*"],
        },
    ];

    fn dropped_command(case: &Dropped) -> Command {
        Command::MovePane {
            pane: id(case.pane),
            placement: at(case.target, case.edge),
        }
    }

    #[test]
    fn dropping_a_tab_on_an_edge_takes_it_out_of_its_strip() {
        for case in DROPPED {
            let mut layout = fixture();
            apply(&mut layout, dropped_command(case));
            assert_eq!(strips(&layout, 1), case.strips, "{}", case.name);
        }
    }

    #[test]
    fn a_pane_alone_in_its_group_cannot_be_dropped_on_its_own_edge() {
        let placement = Placement {
            target: id("d"),
            edge: Edge::Left,
        };
        let command = Command::MovePane {
            pane: id("d"),
            placement,
        };
        refuses(command, &Error::SamePane(id("d")), "own edge");
    }

    #[test]
    fn swapping_exchanges_places_across_groups_and_keeps_the_focused_pane_shown() {
        let mut layout = fixture();
        apply(
            &mut layout,
            Command::SwapPanes {
                a: id("c"),
                b: id("f"),
            },
        );
        assert_eq!(strips(&layout, 1), ["a b f*", "d*", "e c*"], "places");
        assert_eq!(focused(&layout), "c", "focus follows the pane");
    }

    #[test]
    fn swapping_across_groups_leaves_the_same_panes_pinned() {
        let mut layout = fixture();
        apply(&mut layout, Command::PinPane { pane: id("a") });
        let swap = Command::SwapPanes {
            a: id("a"),
            b: id("f"),
        };
        apply(&mut layout, swap);
        let tab = layout.tab(TabId(1)).unwrap();
        let pinned = |name: &str| {
            let pane = id(name);
            tab.group_of(&pane)
                .is_some_and(|group| group.is_pinned(&pane))
        };
        let seen: Vec<bool> = ["a", "b", "c", "e", "f"].map(pinned).into();
        assert_eq!(seen, [true, false, false, false, false], "only a is pinned");
    }

    #[test]
    fn swapping_inside_a_strip_reorders_it() {
        let mut layout = fixture();
        apply(
            &mut layout,
            Command::SwapPanes {
                a: id("a"),
                b: id("c"),
            },
        );
        assert_eq!(strips(&layout, 1), ["c* b a", "d*", "e f*"], "strip");
        assert_eq!(focused(&layout), "c", "focus");
    }

    #[test]
    fn an_editor_tab_cannot_be_detached_without_its_chat() {
        let mut layout = fixture();
        let before = layout.clone();
        let command = Command::DetachPane {
            pane: id("b"),
            new_tab: TabId(9),
            title: "b".into(),
        };
        assert_eq!(layout.apply(command), Err(Error::NotAChat(id("b"))));
        assert_eq!(layout, before, "unchanged");
    }

    #[test]
    fn a_tab_moved_to_another_tab_joins_the_focused_group_there() {
        let mut layout = fixture();
        let command = Command::MovePaneToTab {
            pane: id("b"),
            to: TabId(3),
            placement: None,
        };
        apply(&mut layout, command);
        assert_eq!(strips(&layout, 1), ["a c*", "d*", "e f*"], "source");
        assert_eq!(strips(&layout, 3), ["z b*"], "destination");
    }

    fn undoable() -> Vec<Command> {
        let beside_d = Placement {
            target: id("d"),
            edge: Edge::Bottom,
        };
        vec![
            Command::OpenPaneInGroup {
                instance: pane("n"),
                onto: id("a"),
                index: Some(1),
            },
            Command::MoveToGroup {
                pane: id("b"),
                onto: id("e"),
                index: None,
            },
            Command::MoveToGroup {
                pane: id("a"),
                onto: id("c"),
                index: None,
            },
            Command::ClosePane { pane: id("b") },
            Command::MovePane {
                pane: id("b"),
                placement: beside_d,
            },
        ]
    }

    #[test]
    fn every_strip_command_is_one_undo_step_that_restores_the_strips_and_focus() {
        for command in undoable() {
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
    }

    #[test]
    fn a_move_that_changes_nothing_is_no_undo_step() {
        let mut history = LayoutHistory::new(fixture());
        let command = Command::MoveToGroup {
            pane: id("c"),
            onto: id("a"),
            index: Some(2),
        };
        history.apply(command).unwrap();
        assert!(!history.can_undo(), "nothing to undo");
    }

    #[test]
    fn stacked_layouts_survive_the_stored_form() {
        let layout = fixture();
        let bytes = crate::layout::stored::model::store(&layout).unwrap();
        assert_eq!(
            crate::layout::stored::model::load(&bytes).unwrap(),
            layout,
            "round trip"
        );
    }

    fn stored_group(
        panes: &serde_json::Value,
        active: &str,
        focused: &str,
    ) -> String {
        let terminal = |name: &str| serde_json::json!({"paneId": name, "type": "core.terminal", "params": {"id": name}, "state": null});
        let tree = serde_json::json!({"node": "leaf", "chat": null, "tabs": panes, "active": active,
            "pinned": 0, "editorOpen": true, "chatSelected": false});
        let tab = serde_json::json!({"id": 1, "title": "t", "space": "", "tree": tree,
            "focused": focused, "panes": {"a": terminal("a"), "b": terminal("b")}});
        serde_json::json!({"version": crate::SCHEMA_VERSION, "shown": {"": 1}, "tabs": [tab]})
            .to_string()
    }

    #[test]
    fn a_stored_group_that_breaks_an_invariant_is_refused() {
        let two = serde_json::json!(["a", "b"]);
        let cases = [
            ("fine", stored_group(&two, "a", "a"), true),
            (
                "empty group",
                stored_group(&serde_json::json!([]), "a", "a"),
                false,
            ),
            ("shows a stranger", stored_group(&two, "zz", "a"), false),
            (
                "repeats a pane",
                stored_group(&serde_json::json!(["a", "a", "b"]), "a", "a"),
                false,
            ),
            (
                "misses a pane",
                stored_group(&serde_json::json!(["a"]), "a", "a"),
                false,
            ),
            (
                "focus on a hidden member",
                stored_group(&two, "a", "b"),
                false,
            ),
        ];
        for (name, document, loads) in cases {
            let loaded = crate::layout::stored::model::load(document.as_bytes());
            assert_eq!(loaded.is_ok(), loads, "{name}: {loaded:?}");
        }
    }
}
