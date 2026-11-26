use std::collections::{BTreeMap, VecDeque};

use protocol::PaneId;

use crate::error::Result;
use crate::layout::Layout;
use crate::layout::arrangement::Arrangement;
use crate::layout::command::Command;
use crate::layout::tab::pane_instance::PaneInstance;
use crate::layout::undo_step::UndoStep;
use crate::space_id::SpaceId;
use crate::tab_id::TabId;

pub(crate) const MAX_UNDO_STEPS: usize = 64;

pub(crate) const MAX_PARKED_PANES: usize = 256;

/// A [`Layout`] with undo and redo of structural commands.
///
/// A step stores the arrangement it returns to and the panes that command closed, never a copy
/// of the layout: applying a command costs no more than the command itself, and sending pane
/// state (an editor on every keystroke) neither copies nor records anything. Undo restores the
/// arrangement but keeps the latest state of every pane that is still open, so undoing a close
/// never rolls back work; a pane that was closed comes back with the state it had when it
/// closed. Sizes of splits come back as they were at the step.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct LayoutHistory {
    current: Layout,
    undo: VecDeque<UndoStep>,
    redo: VecDeque<UndoStep>,
}

impl LayoutHistory {
    /// Starts a history at `layout`.
    #[must_use]
    pub fn new(layout: Layout) -> Self {
        Self {
            current: layout,
            undo: VecDeque::new(),
            redo: VecDeque::new(),
        }
    }

    /// The current layout.
    #[must_use]
    pub fn layout(&self) -> &Layout {
        &self.current
    }

    /// Applies a command. A structural command that changed something becomes one undo step
    /// and clears redo.
    ///
    /// # Errors
    /// The [`Error`](crate::Error) that refused the command; history is unchanged then.
    pub fn apply(
        &mut self,
        command: Command,
    ) -> Result<()> {
        let before = command.is_structural().then(|| self.current.arrangement());
        let adopted = match &command {
            Command::AdoptSpace { tab, space } => Some((*tab, space.clone())),
            _ => None,
        };
        let removed = self.current.execute(command)?;
        if let Some((tab, space)) = adopted {
            self.carry_adoption((tab, &space));
        }
        if let Some(before) = before {
            self.record(before, removed);
        }
        Ok(())
    }

    fn record(
        &mut self,
        before: Arrangement,
        removed: Vec<PaneInstance>,
    ) {
        if removed.is_empty() && self.current.arrangement() == before {
            return;
        }
        let parked: BTreeMap<_, _> = removed
            .into_iter()
            .map(|instance| (instance.pane_id.clone(), instance))
            .collect();
        let spaces = before.spaces_changed_in(&self.current.arrangement());
        self.undo.push_back(UndoStep {
            arrangement: before,
            parked,
            spaces,
        });
        self.redo.clear();
        self.trim();
    }

    fn parked_total(&self) -> usize {
        self.undo
            .iter()
            .chain(&self.redo)
            .map(|step| step.parked.len())
            .sum()
    }

    fn trim(&mut self) {
        while self.over_budget() && self.drop_oldest() {}
    }

    fn over_budget(&self) -> bool {
        self.undo.len() > MAX_UNDO_STEPS || self.parked_total() > MAX_PARKED_PANES
    }

    fn drop_oldest(&mut self) -> bool {
        self.undo
            .pop_front()
            .or_else(|| self.redo.pop_front())
            .is_some()
    }

    /// Steps back one structural change. Returns whether anything changed.
    pub fn undo(&mut self) -> bool {
        let Some(step) = self.undo.pop_back() else {
            return false;
        };
        let Ok(inverse) = self.current.swap_arrangement(step) else {
            return false;
        };
        self.redo.push_back(inverse);
        self.trim();
        true
    }

    /// Steps forward again. Returns whether anything changed.
    pub fn redo(&mut self) -> bool {
        let Some(step) = self.redo.pop_back() else {
            return false;
        };
        let Ok(inverse) = self.current.swap_arrangement(step) else {
            return false;
        };
        self.undo.push_back(inverse);
        self.trim();
        true
    }

    /// Takes back the open of `pane`, as if it never happened, when that open is the last
    /// structural change: the layout returns to the arrangement before it, with its focus and
    /// the tab its space showed, and no undo step is left for it. A pane opened earlier closes as any other.
    ///
    /// # Errors
    /// The [`Error`](crate::Error) that refused closing `pane`.
    pub fn withdraw(
        &mut self,
        pane: &PaneId,
    ) -> Result<()> {
        let close = Command::ClosePane { pane: pane.clone() };
        let opened_last = self
            .undo
            .back()
            .is_some_and(|step| !step.arrangement.pane_ids().contains(pane));
        let Some(step) = self.undo.pop_back().filter(|_| opened_last) else {
            return self.apply(close);
        };
        if let Err(step) = self.current.swap_arrangement(step) {
            self.undo.push_back(step);
            return self.apply(close);
        }
        Ok(())
    }

    pub(crate) fn carry_adoption(
        &mut self,
        (tab, space): (TabId, &SpaceId),
    ) {
        for step in self.undo.iter_mut().chain(&mut self.redo) {
            step.arrangement.adopt((tab, space));
            let unowned_changed = step.spaces.contains("");
            step.spaces.extend(unowned_changed.then(|| space.clone()));
        }
    }
}

#[cfg(test)]
mod tests {
    use protocol::PaneState;
    use serde_json::json;

    use super::*;
    use crate::layout::tab::{MAX_PANES_PER_TAB, Tab};
    use crate::tab_id::TabId;
    use crate::test_support::layout_fixtures::{add, instance, open as open_in};
    use crate::test_support::tab_fixtures::{at, id, pane};
    use crate::tree::edge::Edge;

    fn open(name: &str) -> Command {
        open_in(1, pane(name))
    }

    /// Tab 1 holding `a` and `b`, each opening one undo step.
    fn history() -> LayoutHistory {
        let mut history = LayoutHistory::default();
        apply_all(&mut history, vec![add(1), open("a"), open("b")]);
        history
    }

    fn count(history: &LayoutHistory) -> Option<usize> {
        history.layout().tab(TabId(1)).map(Tab::len)
    }

    fn state(value: serde_json::Value) -> PaneState {
        PaneState::new(value).unwrap()
    }

    fn set_state(
        name: &str,
        value: serde_json::Value,
    ) -> Command {
        let (pane, state) = (id(name), state(value));
        Command::SetPaneState { pane, state }
    }

    fn state_of(
        history: &LayoutHistory,
        name: &str,
    ) -> Option<PaneState> {
        history.layout().pane(&id(name)).map(|p| p.state.clone())
    }

    fn apply_all(
        history: &mut LayoutHistory,
        commands: Vec<Command>,
    ) {
        for command in commands {
            history.apply(command).unwrap();
        }
    }

    fn close(name: &str) -> Command {
        Command::ClosePane { pane: id(name) }
    }

    #[test]
    fn undo_and_redo_walk_structural_changes_until_a_new_command() {
        let mut empty = LayoutHistory::new(Layout::default());
        assert!(!empty.undo() && !empty.redo(), "nothing to walk");
        let mut history = history();
        let full = history.layout().clone();
        assert!(history.undo(), "undo open b");
        assert_eq!(count(&history), Some(1), "one pane");
        let mut redone = history.clone();
        assert!(redone.redo(), "redo");
        assert_eq!(redone.layout(), &full, "back to full");
        assert!(!redone.redo(), "nothing left to redo");
        history.apply(open("c")).unwrap();
        assert!(!history.redo(), "a new command clears redo");
        let before = history.clone();
        assert!(history.apply(open("a")).is_err(), "duplicate");
        assert_eq!(history, before, "refused: history unchanged");
    }

    #[test]
    fn only_changes_of_the_arrangement_are_recorded() {
        let move_tab = Command::MoveTab {
            tab: TabId(1),
            to_index: 0,
        };
        let rename = Command::RenameTab {
            tab: TabId(1),
            title: "tab 1".into(),
        };
        let swap = Command::SwapPanes {
            a: id("a"),
            b: id("b"),
        };
        let cases = [
            (Command::FocusPane { pane: id("a") }, 0),
            (Command::ActivateTab { tab: TabId(1) }, 0),
            (move_tab, 0),
            (rename, 0),
            (set_state("a", json!({"draft": "typed"})), 0),
            (swap, 1),
        ];
        for (command, recorded) in cases {
            let mut history = history();
            let steps = history.undo.len();
            history.apply(command.clone()).unwrap();
            history.apply(command.clone()).unwrap();
            let grown = history.undo.len() - steps;
            assert_eq!(grown, 2 * recorded, "{command:?}");
            assert_eq!(history.parked_total(), 0, "{command:?}: none parked");
        }
    }

    #[test]
    fn undoing_a_close_restores_its_state_and_keeps_later_work() {
        let mut history = history();
        history
            .apply(set_state("b", json!({"draft": "unsent"})))
            .unwrap();
        history.apply(close("b")).unwrap();
        history.apply(set_state("a", json!({"scroll": 7}))).unwrap();
        assert_eq!(count(&history), Some(1), "closed");
        assert!(history.undo(), "undo the close");
        let closed = Some(state(json!({"draft": "unsent"})));
        assert_eq!(state_of(&history, "b"), closed, "state at close");
        let open = Some(state(json!({"scroll": 7})));
        assert_eq!(state_of(&history, "a"), open, "work since survives");
        history.layout().validate().unwrap();
    }

    #[test]
    fn a_change_that_closes_or_spans_tabs_is_one_undo_step() {
        let detach = Command::DetachPane {
            pane: id("b"),
            new_tab: TabId(2),
            title: "b".into(),
        };
        let move_to_tab = Command::MovePaneToTab {
            pane: id("a"),
            to: TabId(2),
            placement: None,
        };
        let cases = [
            (vec![], detach, "b", 2),
            (vec![add(2)], move_to_tab, "a", 2),
            (vec![add(2), close("b")], close("a"), "a", 1),
        ];
        for (prelude, command, moved, tabs) in cases {
            let mut history = history();
            apply_all(&mut history, prelude);
            let before = history.layout().clone();
            history.apply(command.clone()).unwrap();
            let after = history.layout().clone();
            assert_eq!(after.tabs().len(), tabs, "{command:?}: tabs");
            let holder = after.tab_of(&id(moved)).map(Tab::id);
            assert_ne!(holder, Some(TabId(1)), "{command:?}: left tab 1");
            assert!(history.undo(), "{command:?}: one undo");
            assert_eq!(history.layout(), &before, "{command:?}: back");
            assert!(history.redo(), "{command:?}: redo");
            assert_eq!(history.layout(), &after, "{command:?}: again");
        }
    }

    #[test]
    fn history_is_bounded_in_steps() {
        let mut history = LayoutHistory::default();
        history.apply(add(1)).unwrap();
        for index in 0..(MAX_UNDO_STEPS + 40) {
            let title = format!("title {index}");
            history
                .apply(Command::RenameTab {
                    tab: TabId(1),
                    title,
                })
                .unwrap();
        }
        assert_eq!(history.undo.len(), MAX_UNDO_STEPS, "oldest forgotten");
        let undone = std::iter::from_fn(|| history.undo().then_some(())).count();
        assert_eq!(undone, MAX_UNDO_STEPS, "exactly the kept steps");
    }

    #[test]
    fn history_is_bounded_in_closed_panes_it_keeps_alive() {
        let tabs = 5_u64;
        let mut history = LayoutHistory::default();
        for tab in 0..tabs {
            history.apply(add(tab)).unwrap();
            let names = (0..MAX_PANES_PER_TAB).map(|pane| format!("p{tab}-{pane}"));
            let opens = names.map(|name| open_in(tab, pane(&name)));
            apply_all(&mut history, opens.collect());
        }
        let mut history = LayoutHistory::new(history.layout().clone());
        let closes = (0..tabs).map(|tab| Command::CloseTab { tab: TabId(tab) });
        apply_all(&mut history, closes.collect());
        let parked = history.parked_total();
        assert!(parked <= MAX_PARKED_PANES, "{parked}");
        let kept = MAX_PARKED_PANES / MAX_PANES_PER_TAB;
        assert_eq!(history.undo.len(), kept, "oldest closes forgotten");
        assert!(history.undo(), "the newest close is still undoable");
    }

    #[test]
    fn a_step_that_can_no_longer_be_applied_is_dropped_not_half_applied() {
        let mut history = history();
        history.apply(close("b")).unwrap();
        history.undo.back_mut().unwrap().parked.clear();
        let before = history.layout().clone();
        let steps = history.undo.len();
        assert!(!history.undo(), "cannot restore a pane that is gone");
        assert_eq!(history.layout(), &before, "layout untouched");
        assert_eq!(history.undo.len(), steps - 1, "the broken step is gone");
    }

    #[test]
    fn undo_restores_focus_and_the_active_tab() {
        let mut history = history();
        history.apply(add(2)).unwrap();
        history.apply(Command::FocusPane { pane: id("a") }).unwrap();
        let placement = at("a", Edge::Left);
        let moving = Command::MovePane {
            pane: id("b"),
            placement,
        };
        history.apply(moving).unwrap();
        assert!(history.undo(), "undo the move");
        let layout = history.layout();
        assert_eq!(layout.showing("").map(Tab::id), Some(TabId(1)), "tab");
        let focused = layout.tab(TabId(1)).and_then(Tab::focused);
        assert_eq!(focused, Some(&id("a")), "focus");
    }

    fn add_in(
        id: u64,
        space: &str,
    ) -> Command {
        let (id, space) = (TabId(id), SpaceId::new(space));
        let title = String::new();
        Command::AddTab { id, title, space }
    }

    fn adopt(space: &str) -> Command {
        let (tab, space) = (TabId(1), SpaceId::new(space));
        Command::AdoptSpace { tab, space }
    }

    fn space_of_first(history: &LayoutHistory) -> Option<&str> {
        history.layout().tab(TabId(1)).map(Tab::space)
    }

    #[test]
    fn an_unowned_tab_is_adopted_once_and_undo_and_redo_keep_its_space() {
        let mut history = LayoutHistory::default();
        let steps = [add_in(1, ""), open_in(1, instance("a")), add_in(2, "p1"), open_in(2, instance("b"))];
        for step in steps.into_iter().chain([Command::ActivateTab { tab: TabId(1) }]) {
            history.apply(step).unwrap();
        }
        history.apply(adopt("p2")).unwrap();
        let layout = history.layout();
        assert_eq!(space_of_first(&history), Some("p2"), "adopted");
        assert_eq!(layout.showing("p2").map(Tab::id), Some(TabId(1)), "shown");
        assert!(layout.showing("").is_none(), "the empty space shows none");
        assert!(history.apply(adopt("p3")).is_err(), "only once");
        assert!(history.undo() && history.undo(), "undo the second tab");
        assert_eq!(space_of_first(&history), Some("p2"), "undo keeps it");
        assert!(history.redo(), "redo");
        assert_eq!(space_of_first(&history), Some("p2"), "redo keeps it");
        history.layout().validate().unwrap();
    }

    fn history_of(after: &[&str]) -> LayoutHistory {
        let mut history = LayoutHistory::default();
        let opens = ["a", "b"].iter().chain(after);
        let opens = opens.map(|name| open_in(1, instance(name)));
        let focus = Command::FocusPane { pane: id("a") };
        let mut commands: Vec<Command> = std::iter::once(add(1)).chain(opens).collect();
        commands.insert(3, focus);
        for command in commands {
            history.apply(command).unwrap();
        }
        history
    }

    #[test]
    fn withdrawing_returns_to_the_focus_before_the_open_or_closes_an_older_pane() {
        let cases: [(&[&str], &str, bool); 2] = [(&["c"], "a", false), (&["c", "d"], "d", true)];
        for (after, focused, undo_brings_it_back) in cases {
            let mut history = history_of(after);
            history.withdraw(&id("c")).unwrap();
            let tab = history.layout().tab(TabId(1)).unwrap();
            assert_eq!(tab.focused(), Some(&id(focused)), "{after:?}: focus");
            assert!(history.layout().pane(&id("c")).is_none(), "{after:?}: gone");
            history.undo();
            let back = history.layout().pane(&id("c")).is_some();
            assert_eq!(back, undo_brings_it_back, "{after:?}: undo");
        }
    }
}
