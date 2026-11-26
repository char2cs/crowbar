//! Public-API property tests: random command sequences never break an invariant, refused
//! commands change nothing, panes are never lost or duplicated, and undo is exact.

use std::collections::{BTreeMap, BTreeSet};

use layout::{
    Branch, Command, Edge, Layout, LayoutHistory, PaneInstance, Placement, Rect, SpaceId, Tab,
    TabId,
};
use proptest::prelude::*;
use protocol::{PaneId, PaneState, PaneTypeId, Params};
use serde_json::json;

const TABS: u8 = 4;
const PANES: u8 = 8;
const CHATS: u8 = 3;
const SCREEN: Rect = Rect {
    x: 0.0,
    y: 0.0,
    width: 2000.0,
    height: 1000.0,
};

#[derive(Debug, Clone)]
enum Step {
    AddTab(u8),
    CloseTab(u8),
    Activate(u8),
    MoveTab(u8, u8),
    Rename(u8),
    Open {
        tab: u8,
        pane: u8,
        chat: Option<u8>,
        placement: Option<(u8, Edge)>,
    },
    OpenInGroup {
        pane: u8,
        chat: Option<u8>,
        onto: u8,
        index: Option<u8>,
    },
    JoinGroup(u8, u8, Option<u8>),
    Close(u8),
    Focus(u8),
    MovePane(u8, u8, Edge),
    MoveToTab(u8, u8, Option<(u8, Edge)>),
    Detach(u8, u8),
    Swap(u8, u8),
    Resize(Vec<Branch>, f32),
    Reset(Vec<Branch>),
    SetState(u8, u8),
}

fn edge() -> impl Strategy<Value = Edge> {
    prop_oneof![
        Just(Edge::Left),
        Just(Edge::Right),
        Just(Edge::Top),
        Just(Edge::Bottom)
    ]
}

fn branch() -> impl Strategy<Value = Branch> {
    prop_oneof![Just(Branch::First), Just(Branch::Second)]
}

fn step() -> impl Strategy<Value = Step> {
    let tab = 0..TABS;
    let pane = 0..PANES;
    prop_oneof![
        2 => tab.clone().prop_map(Step::AddTab),
        2 => tab.clone().prop_map(Step::CloseTab),
        1 => tab.clone().prop_map(Step::Activate),
        1 => (tab.clone(), 0..TABS).prop_map(|(t, i)| Step::MoveTab(t, i)),
        1 => tab.clone().prop_map(Step::Rename),
        6 => (tab.clone(), pane.clone(), proptest::option::of(0..CHATS), proptest::option::of((0..PANES, edge())))
            .prop_map(|(tab, pane, chat, placement)| Step::Open { tab, pane, chat, placement }),
        5 => (pane.clone(), proptest::option::of(0..CHATS), 0..PANES, proptest::option::of(0..4_u8))
            .prop_map(|(pane, chat, onto, index)| Step::OpenInGroup { pane, chat, onto, index }),
        4 => (pane.clone(), 0..PANES, proptest::option::of(0..4_u8))
            .prop_map(|(p, onto, index)| Step::JoinGroup(p, onto, index)),
        3 => pane.clone().prop_map(Step::Close),
        1 => pane.clone().prop_map(Step::Focus),
        2 => (pane.clone(), 0..PANES, edge()).prop_map(|(p, t, e)| Step::MovePane(p, t, e)),
        2 => (pane.clone(), tab.clone(), proptest::option::of((0..PANES, edge()))).prop_map(|(p, t, pl)| Step::MoveToTab(p, t, pl)),
        2 => (pane.clone(), tab.clone()).prop_map(|(p, t)| Step::Detach(p, t)),
        1 => (pane.clone(), 0..PANES).prop_map(|(a, b)| Step::Swap(a, b)),
        1 => (proptest::collection::vec(branch(), 0..3), -100.0_f32..2200.0).prop_map(|(p, e)| Step::Resize(p, e)),
        1 => proptest::collection::vec(branch(), 0..3).prop_map(Step::Reset),
        1 => (pane, 0..8_u8).prop_map(|(p, s)| Step::SetState(p, s)),
    ]
}

fn sequence() -> impl Strategy<Value = Vec<Step>> {
    proptest::collection::vec(step(), 1..60)
}

type Fixture<T> = Result<T, Box<dyn std::error::Error>>;

fn failed(error: &dyn std::error::Error) -> TestCaseError {
    TestCaseError::fail(error.to_string())
}

fn pane_id(n: u8) -> Fixture<PaneId> {
    Ok(PaneId::new(format!("p{n}"))?)
}

fn tab_id(n: u8) -> TabId {
    TabId(u64::from(n))
}

fn placement((target, edge): (u8, Edge)) -> Fixture<Placement> {
    Ok(Placement {
        target: pane_id(target)?,
        edge,
    })
}

fn instance(
    pane: u8,
    chat: Option<u8>,
) -> Fixture<PaneInstance> {
    let (type_id, params) = match chat {
        Some(chat) => (
            "core.chat",
            Params::default().with("chatId", format!("c{chat}"))?,
        ),
        None => ("core.terminal", Params::default()),
    };
    Ok(PaneInstance {
        pane_id: pane_id(pane)?,
        type_id: PaneTypeId::new(type_id)?,
        params,
        state: PaneState::default(),
    })
}

fn command(step: &Step) -> Fixture<Command> {
    match tab_command(step) {
        Some(command) => Ok(command),
        None => pane_command(step),
    }
}

fn tab_command(step: &Step) -> Option<Command> {
    let command = match step.clone() {
        Step::AddTab(t) => Command::AddTab {
            id: tab_id(t),
            title: format!("tab {t}"),
            space: SpaceId::default(),
        },
        Step::CloseTab(t) => Command::CloseTab { tab: tab_id(t) },
        Step::Activate(t) => Command::ActivateTab { tab: tab_id(t) },
        Step::MoveTab(t, i) => Command::MoveTab {
            tab: tab_id(t),
            to_index: usize::from(i),
        },
        Step::Rename(t) => Command::RenameTab {
            tab: tab_id(t),
            title: format!("renamed {t}"),
        },
        Step::Resize(path, extent) => Command::ResizeSplit {
            tab: tab_id(0),
            path,
            bounds: SCREEN,
            first_extent: extent,
        },
        Step::Reset(path) => Command::ResetSplit {
            tab: tab_id(0),
            path,
        },
        _ => return None,
    };
    Some(command)
}

fn group_command(step: &Step) -> Fixture<Option<Command>> {
    let command = match step.clone() {
        Step::Open {
            tab,
            pane,
            chat,
            placement: at,
        } => Command::OpenPane {
            tab: tab_id(tab),
            instance: instance(pane, chat)?,
            placement: at.map(placement).transpose()?,
        },
        Step::OpenInGroup {
            pane,
            chat,
            onto,
            index,
        } => Command::OpenPaneInGroup {
            instance: instance(pane, chat)?,
            onto: pane_id(onto)?,
            index: index.map(usize::from),
        },
        Step::JoinGroup(p, onto, index) => Command::MoveToGroup {
            pane: pane_id(p)?,
            onto: pane_id(onto)?,
            index: index.map(usize::from),
        },
        _ => return Ok(None),
    };
    Ok(Some(command))
}

fn pane_command(step: &Step) -> Fixture<Command> {
    match group_command(step)? {
        Some(command) => Ok(command),
        None => plain_pane_command(step),
    }
}

fn plain_pane_command(step: &Step) -> Fixture<Command> {
    let command = match step.clone() {
        Step::Close(p) => Command::ClosePane { pane: pane_id(p)? },
        Step::Focus(p) => Command::FocusPane { pane: pane_id(p)? },
        Step::MovePane(p, t, e) => Command::MovePane {
            pane: pane_id(p)?,
            placement: placement((t, e))?,
        },
        Step::MoveToTab(p, t, at) => Command::MovePaneToTab {
            pane: pane_id(p)?,
            to: tab_id(t),
            placement: at.map(placement).transpose()?,
        },
        Step::Detach(p, t) => Command::DetachPane {
            pane: pane_id(p)?,
            new_tab: tab_id(t),
            title: "detached".into(),
        },
        Step::Swap(a, b) => Command::SwapPanes {
            a: pane_id(a)?,
            b: pane_id(b)?,
        },
        Step::SetState(p, s) => Command::SetPaneState {
            pane: pane_id(p)?,
            state: PaneState::new(json!({ "s": s }))?,
        },
        other => unreachable!("{other:?} is a tab step"),
    };
    Ok(command)
}

fn panes(layout: &Layout) -> BTreeSet<PaneId> {
    layout.tabs().iter().flat_map(Tab::pane_ids).collect()
}

fn empty_tabs(layout: &Layout) -> BTreeSet<TabId> {
    layout
        .tabs()
        .iter()
        .filter(|t| t.is_empty())
        .map(Tab::id)
        .collect()
}

fn identities(layout: &Layout) -> BTreeMap<PaneId, (PaneTypeId, Params)> {
    panes(layout)
        .into_iter()
        .filter_map(|id| {
            layout
                .pane(&id)
                .map(|p| (id, (p.type_id.clone(), p.params.clone())))
        })
        .collect()
}

fn states(layout: &Layout) -> BTreeMap<PaneId, PaneState> {
    panes(layout)
        .into_iter()
        .filter_map(|id| layout.pane(&id).map(|p| (id, p.state.clone())))
        .collect()
}

fn closed_by(
    before: &Layout,
    closed: &PaneId,
) -> BTreeSet<PaneId> {
    let Some(tab) = before.tab_of(closed) else {
        return BTreeSet::from([closed.clone()]);
    };
    let is_chat = |id: &PaneId| tab.pane(id).is_some_and(|p| p.type_id.is_chat());
    let chats = tab.pane_ids().into_iter().filter(|id| is_chat(id)).count();
    if is_chat(closed) && chats == 1 {
        return tab.pane_ids().into_iter().collect();
    }
    BTreeSet::from([closed.clone()])
}

fn check_transition(
    step: &Step,
    before: &Layout,
    after: &Layout,
) -> Fixture<()> {
    let (was, now) = (panes(before), panes(after));
    match step {
        Step::Close(p) => {
            let gone = closed_by(before, &pane_id(*p)?);
            let expected: BTreeSet<_> = was.difference(&gone).cloned().collect();
            assert_eq!(now, expected, "{step:?}: exactly the closed panes are gone");
        },
        Step::CloseTab(t) => {
            let gone: BTreeSet<_> = before
                .tab(tab_id(*t))
                .map_or_else(BTreeSet::new, |tab| tab.pane_ids().into_iter().collect());
            let expected: BTreeSet<_> = was.difference(&gone).cloned().collect();
            assert_eq!(now, expected, "{step:?}: exactly the tab's panes are gone");
        },
        Step::Open { .. } | Step::OpenInGroup { .. } => {
            assert!(
                was.is_subset(&now) && now.len() <= was.len() + 1,
                "{step:?}: opens at most one pane and loses none"
            );
        },
        _ => assert_eq!(now, was, "{step:?}: no pane appears or disappears"),
    }
    check_what_stayed(step, before, after);
    check_focus(step, after);
    Ok(())
}

fn check_what_stayed(
    step: &Step,
    before: &Layout,
    after: &Layout,
) {
    let mut allowed = empty_tabs(before);
    if let Step::AddTab(t) = step {
        allowed.insert(tab_id(*t));
    }
    assert!(
        empty_tabs(after).is_subset(&allowed),
        "{step:?}: a command left an empty tab behind"
    );
    let (old, new) = (identities(before), identities(after));
    for (id, identity) in &old {
        assert!(
            new.get(id).is_none_or(|n| n == identity),
            "{step:?}: {id} changed what it is"
        );
    }
    let (old_state, new_state) = (states(before), states(after));
    for (id, state) in &old_state {
        let touched =
            matches!(step, Step::SetState(p, _) if pane_id(*p).is_ok_and(|named| named == *id));
        assert!(
            touched || new_state.get(id).is_none_or(|n| n == state),
            "{step:?}: {id} lost its state"
        );
    }
}

fn check_focus(
    step: &Step,
    after: &Layout,
) {
    let shown = |pane: u8| {
        pane_id(pane).is_ok_and(|id| after.showing("").is_some_and(|t| t.focused() == Some(&id)))
    };
    match step {
        Step::Focus(p)
        | Step::Detach(p, _)
        | Step::MoveToTab(p, _, _)
        | Step::JoinGroup(p, _, _) => {
            assert!(shown(*p), "{step:?}: the pane is shown and focused");
        },
        _ => {},
    }
    for tab in after.tabs() {
        assert_eq!(
            tab.focused().is_some(),
            !tab.is_empty(),
            "{step:?}: focus iff panes"
        );
    }
}

fn run(steps: &[Step]) -> Fixture<Layout> {
    let mut layout = Layout::default();
    for step in steps {
        let before = layout.clone();
        match layout.apply(command(step)?) {
            Ok(()) => check_transition(step, &before, &layout)?,
            Err(_) => assert_eq!(
                layout, before,
                "{step:?}: a refused command changed the layout"
            ),
        }
        let valid = layout.validate();
        assert!(valid.is_ok(), "{step:?}: {valid:?}");
    }
    Ok(layout)
}

proptest! {
    #![proptest_config(ProptestConfig::with_cases(256))]

    #[test]
    fn random_sequences_keep_every_invariant(steps in sequence()) {
        let layout = run(&steps).map_err(|error| failed(&*error))?;
        let json = serde_json::to_value(&layout).unwrap();
        let loaded: Layout = serde_json::from_value(json).unwrap();
        prop_assert_eq!(loaded, layout, "a valid layout survives a round trip through its stored form");
    }

    #[test]
    fn undoing_everything_empties_the_layout_and_redoing_restores_it(steps in sequence()) {
        let mut history = LayoutHistory::default();
        for step in &steps {
            history.apply(command(step).map_err(|error| failed(&*error))?).ok();
        }
        let last = history.layout().clone();
        while history.undo() {
            history.layout().validate().unwrap();
        }
        prop_assert_eq!(history.layout(), &Layout::default());
        while history.redo() {
            history.layout().validate().unwrap();
        }
        prop_assert_eq!(history.layout(), &last);
    }

    #[test]
    fn each_undo_returns_to_the_layout_before_that_step(steps in sequence()) {
        let structural: Vec<Step> = steps
            .into_iter()
            .filter(|s| !matches!(s, Step::Activate(_) | Step::Focus(_) | Step::Resize(..) | Step::Reset(_) | Step::SetState(..)))
            .collect();
        let mut history = LayoutHistory::default();
        let mut before_each = Vec::new();
        for step in &structural {
            let before = history.layout().clone();
            let applied = history.apply(command(step).map_err(|error| failed(&*error))?).is_ok();
            let changed = history.layout() != &before;
            before_each.extend((applied && changed).then_some(before));
        }
        for before in before_each.iter().rev() {
            prop_assert!(history.undo());
            prop_assert_eq!(history.layout(), before);
        }
    }

    #[test]
    fn panes_never_overlap_or_leave_the_screen(steps in sequence()) {
        let layout = run(&steps).map_err(|error| failed(&*error))?;
        for tab in layout.tabs() {
            let slots = tab.layout(SCREEN);
            prop_assert!(slots.iter().all(|slot| on_screen(slot.rect)), "a pane left the screen");
            prop_assert!(first_overlap(&slots).is_none(), "{:?}", first_overlap(&slots));
        }
    }
}

fn on_screen(r: Rect) -> bool {
    let inside_x = r.x >= -0.01 && r.x + r.width <= SCREEN.width + 0.01;
    let inside_y = r.y >= -0.01 && r.y + r.height <= SCREEN.height + 0.01;
    r.width >= 0.0 && r.height >= 0.0 && inside_x && inside_y
}

fn apart(
    a: Rect,
    b: Rect,
) -> bool {
    a.x + a.width <= b.x + 0.01
        || b.x + b.width <= a.x + 0.01
        || a.y + a.height <= b.y + 0.01
        || b.y + b.height <= a.y + 0.01
}

fn first_overlap(slots: &[layout::PaneSlot]) -> Option<String> {
    slots.iter().enumerate().find_map(|(i, a)| {
        slots
            .iter()
            .skip(i + 1)
            .find(|b| !apart(a.rect, b.rect))
            .map(|b| format!("{a:?} overlaps {b:?}"))
    })
}

fn type_into_pane(
    history: &mut LayoutHistory,
    keystroke: u32,
) -> Fixture<()> {
    let draft = "x".repeat(usize::try_from(keystroke % 512)?);
    let command = Command::SetPaneState {
        pane: pane_id(2)?,
        state: PaneState::new(json!({ "draft": draft }))?,
    };
    history.apply(command)?;
    Ok(())
}

#[test]
fn state_sent_per_keystroke_never_grows_history_or_copies_the_layout() -> Fixture<()> {
    let mut history = LayoutHistory::default();
    history.apply(command(&Step::AddTab(0))?)?;
    for pane in 0..4 {
        history.apply(command(&open_step(pane))?)?;
    }
    for keystroke in 0..2000_u32 {
        type_into_pane(&mut history, keystroke)?;
    }
    let mut undone = 0;
    while history.undo() {
        undone += 1;
    }
    assert_eq!(undone, 5, "only the five structural commands are undoable");
    Ok(())
}

#[test]
fn commands_naming_nothing_that_exists_are_refused_and_change_nothing() -> Fixture<()> {
    let cases = [
        (
            "closing an unknown tab",
            Command::CloseTab { tab: tab_id(1) },
        ),
        (
            "activating an unknown tab",
            Command::ActivateTab { tab: tab_id(1) },
        ),
        (
            "closing an unknown pane",
            Command::ClosePane { pane: pane_id(1)? },
        ),
        (
            "focusing an unknown pane",
            Command::FocusPane { pane: pane_id(1)? },
        ),
    ];
    for (name, refused) in cases {
        let mut layout = Layout::default();
        assert!(layout.apply(refused).is_err(), "{name}");
        assert_eq!(layout, Layout::default(), "{name}");
    }
    Ok(())
}

fn open_step(pane: u8) -> Step {
    Step::Open {
        tab: 0,
        pane,
        chat: None,
        placement: None,
    }
}

fn add_in(
    tab: u8,
    space: &str,
) -> Command {
    Command::AddTab {
        id: tab_id(tab),
        title: format!("tab {tab}"),
        space: SpaceId::new(space),
    }
}

fn open_in(
    tab: u8,
    pane: u8,
) -> Fixture<Command> {
    command(&Step::Open {
        tab,
        pane,
        chat: None,
        placement: None,
    })
}

fn adopt(
    tab: u8,
    space: &str,
) -> Command {
    Command::AdoptSpace {
        tab: tab_id(tab),
        space: SpaceId::new(space),
    }
}

fn shown_in(
    layout: &Layout,
    space: &str,
) -> Option<TabId> {
    layout.showing(space).map(Tab::id)
}

fn two_spaces() -> Fixture<Layout> {
    let mut layout = Layout::default();
    for step in [
        add_in(1, "p1"),
        open_in(1, 1)?,
        add_in(2, "p2"),
        open_in(2, 2)?,
    ] {
        layout.apply(step)?;
    }
    Ok(layout)
}

#[test]
fn a_tab_shown_in_one_space_is_not_shown_in_another() -> Fixture<()> {
    let mut layout = two_spaces()?;
    assert_eq!(shown_in(&layout, "p1"), Some(tab_id(1)), "p1 shows its tab");
    assert_eq!(shown_in(&layout, "p2"), Some(tab_id(2)), "p2 shows its tab");
    assert_eq!(shown_in(&layout, "p3"), None, "p3 shows its stage");
    assert_eq!(
        layout.tab(tab_id(2)).map(Tab::space),
        Some("p2"),
        "made in p2"
    );
    layout.apply(Command::FocusPane { pane: pane_id(2)? })?;
    assert_eq!(
        shown_in(&layout, "p1"),
        Some(tab_id(1)),
        "focus stays in p2"
    );
    layout.apply(Command::CloseTab { tab: tab_id(1) })?;
    assert_eq!(shown_in(&layout, "p1"), None, "p1 falls back to its stage");
    assert_eq!(shown_in(&layout, "p2"), Some(tab_id(2)), "p2 untouched");
    layout.validate()?;
    Ok(())
}

#[test]
fn closing_a_shown_tab_hands_over_within_its_own_space() -> Fixture<()> {
    let mut layout = two_spaces()?;
    for step in [
        add_in(3, "p1"),
        open_in(3, 3)?,
        add_in(4, "p2"),
        open_in(4, 4)?,
    ] {
        layout.apply(step)?;
    }
    layout.apply(Command::CloseTab { tab: tab_id(3) })?;
    assert_eq!(shown_in(&layout, "p1"), Some(tab_id(1)), "p1's other tab");
    assert_eq!(shown_in(&layout, "p2"), Some(tab_id(4)), "p2 untouched");
    Ok(())
}

#[test]
fn withdrawing_an_open_restores_the_shown_tab_only_of_the_space_it_changed() -> Fixture<()> {
    let mut history = LayoutHistory::default();
    for step in [
        add_in(1, "p1"),
        open_in(1, 1)?,
        add_in(2, "p1"),
        open_in(2, 2)?,
        add_in(3, "p2"),
        open_in(3, 3)?,
        add_in(4, "p2"),
        open_in(4, 4)?,
        Command::ActivateTab { tab: tab_id(1) },
        open_in(2, 5)?,
    ] {
        history.apply(step)?;
    }
    history.apply(Command::ActivateTab { tab: tab_id(3) })?;
    history.withdraw(&pane_id(5)?)?;
    let layout = history.layout();
    assert_eq!(
        shown_in(layout, "p1"),
        Some(tab_id(1)),
        "p1 back where it was"
    );
    assert_eq!(
        shown_in(layout, "p2"),
        Some(tab_id(3)),
        "p2 keeps the later switch"
    );
    layout.validate()?;
    Ok(())
}

#[test]
fn a_space_with_tabs_and_none_remembered_shows_its_first_tab_in_strip_order() -> Fixture<()> {
    let mut layout = Layout::default();
    for step in [add_in(1, ""), add_in(2, ""), add_in(3, "")] {
        layout.apply(step)?;
    }
    layout.apply(Command::ActivateTab { tab: tab_id(3) })?;
    for (tab, space) in [(3, "p2"), (2, "p1"), (1, "p1")] {
        layout.apply(adopt(tab, space))?;
    }
    assert_eq!(
        shown_in(&layout, "p2"),
        Some(tab_id(3)),
        "p2 remembers its tab"
    );
    assert_eq!(
        shown_in(&layout, "p1"),
        Some(tab_id(1)),
        "p1 falls back to its first tab"
    );
    assert_eq!(
        shown_in(&layout, "p3"),
        None,
        "a space with no tabs shows its stage"
    );
    Ok(())
}
