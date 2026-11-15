//! Public-API property tests: random command sequences never break an invariant, refused
//! commands change nothing, panes are never lost or duplicated, and undo is exact.

#![allow(
    clippy::unwrap_used,
    reason = "test: a failing fixture should fail loudly"
)]

use std::collections::{BTreeMap, BTreeSet};

use model::{
    Branch, Command, Edge, Layout, LayoutHistory, PaneInstance, Placement, Rect, Tab, TabId,
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

fn pane_id(n: u8) -> PaneId {
    PaneId::new(format!("p{n}")).unwrap()
}

fn tab_id(n: u8) -> TabId {
    TabId(u64::from(n))
}

fn placement((target, edge): (u8, Edge)) -> Placement {
    Placement {
        target: pane_id(target),
        edge,
    }
}

fn instance(
    pane: u8,
    chat: Option<u8>,
) -> PaneInstance {
    let (type_id, params) = match chat {
        Some(chat) => (
            "core.chat",
            Params::default()
                .with("chatId", format!("c{chat}"))
                .unwrap(),
        ),
        None => ("core.terminal", Params::default()),
    };
    PaneInstance {
        pane_id: pane_id(pane),
        type_id: PaneTypeId::new(type_id).unwrap(),
        params,
        state: PaneState::default(),
    }
}

fn command(step: &Step) -> Command {
    tab_command(step).unwrap_or_else(|| pane_command(step))
}

fn tab_command(step: &Step) -> Option<Command> {
    let command = match step.clone() {
        Step::AddTab(t) => Command::AddTab {
            id: tab_id(t),
            title: format!("tab {t}"),
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

fn pane_command(step: &Step) -> Command {
    match step.clone() {
        Step::Open {
            tab,
            pane,
            chat,
            placement: at,
        } => Command::OpenPane {
            tab: tab_id(tab),
            instance: instance(pane, chat),
            placement: at.map(placement),
        },
        Step::Close(p) => Command::ClosePane { pane: pane_id(p) },
        Step::Focus(p) => Command::FocusPane { pane: pane_id(p) },
        Step::MovePane(p, t, e) => Command::MovePane {
            pane: pane_id(p),
            placement: placement((t, e)),
        },
        Step::MoveToTab(p, t, at) => Command::MovePaneToTab {
            pane: pane_id(p),
            to: tab_id(t),
            placement: at.map(placement),
        },
        Step::Detach(p, t) => Command::DetachPane {
            pane: pane_id(p),
            new_tab: tab_id(t),
            title: "detached".into(),
        },
        Step::Swap(a, b) => Command::SwapPanes {
            a: pane_id(a),
            b: pane_id(b),
        },
        Step::SetState(p, s) => Command::SetPaneState {
            pane: pane_id(p),
            state: PaneState::new(json!({ "s": s })).unwrap(),
        },
        other => unreachable!("{other:?} is a tab step"),
    }
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

fn check_transition(
    step: &Step,
    before: &Layout,
    after: &Layout,
) {
    let (was, now) = (panes(before), panes(after));
    match step {
        Step::Close(p) => {
            let expected: BTreeSet<_> = was
                .iter()
                .filter(|id| **id != pane_id(*p))
                .cloned()
                .collect();
            assert_eq!(now, expected, "{step:?}: exactly the closed pane is gone");
        },
        Step::CloseTab(t) => {
            let gone: BTreeSet<_> = before
                .tab(tab_id(*t))
                .map_or_else(BTreeSet::new, |tab| tab.pane_ids().into_iter().collect());
            let expected: BTreeSet<_> = was.difference(&gone).cloned().collect();
            assert_eq!(now, expected, "{step:?}: exactly the tab's panes are gone");
        },
        Step::Open { .. } => {
            assert!(
                was.is_subset(&now) && now.len() <= was.len() + 1,
                "{step:?}: opens at most one pane and loses none"
            );
        },
        _ => assert_eq!(now, was, "{step:?}: no pane appears or disappears"),
    }
    check_what_stayed(step, before, after);
    check_focus(step, after);
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
        let touched = matches!(step, Step::SetState(p, _) if pane_id(*p) == *id);
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
        let id = pane_id(pane);
        after.active_tab().is_some_and(|t| t.focused() == Some(&id))
    };
    match step {
        Step::Focus(p) | Step::Detach(p, _) | Step::MoveToTab(p, _, _) => {
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

fn run(steps: &[Step]) -> Layout {
    let mut layout = Layout::default();
    for step in steps {
        let before = layout.clone();
        match layout.apply(command(step)) {
            Ok(()) => check_transition(step, &before, &layout),
            Err(_) => assert_eq!(
                layout, before,
                "{step:?}: a refused command changed the layout"
            ),
        }
        let valid = layout.validate();
        assert!(valid.is_ok(), "{step:?}: {valid:?}");
    }
    layout
}

proptest! {
    #![proptest_config(ProptestConfig::with_cases(256))]

    #[test]
    fn random_sequences_keep_every_invariant(steps in sequence()) {
        let layout = run(&steps);
        let json = serde_json::to_value(&layout).unwrap();
        let loaded: Layout = serde_json::from_value(json).unwrap();
        prop_assert_eq!(loaded, layout, "a valid layout survives a round trip through its stored form");
    }

    #[test]
    fn undoing_everything_empties_the_layout_and_redoing_restores_it(steps in sequence()) {
        let mut history = LayoutHistory::default();
        for step in &steps {
            history.apply(command(step)).ok();
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
            let applied = history.apply(command(step)).is_ok();
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
        let layout = run(&steps);
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

fn first_overlap(slots: &[model::PaneSlot]) -> Option<String> {
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
) {
    let draft = "x".repeat(usize::try_from(keystroke % 512).unwrap());
    let command = Command::SetPaneState {
        pane: pane_id(2),
        state: PaneState::new(json!({ "draft": draft })).unwrap(),
    };
    history.apply(command).unwrap();
}

#[test]
fn state_sent_per_keystroke_never_grows_history_or_copies_the_layout() {
    let mut history = LayoutHistory::default();
    history.apply(command(&Step::AddTab(0))).unwrap();
    for pane in 0..4 {
        history.apply(command(&open_step(pane))).unwrap();
    }
    for keystroke in 0..2000_u32 {
        type_into_pane(&mut history, keystroke);
    }
    let mut undone = 0;
    while history.undo() {
        undone += 1;
    }
    assert_eq!(undone, 5, "only the five structural commands are undoable");
}

fn open_step(pane: u8) -> Step {
    Step::Open {
        tab: 0,
        pane,
        chat: None,
        placement: None,
    }
}
