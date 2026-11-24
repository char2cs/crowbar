use std::collections::{BTreeMap, VecDeque};

use crate::arrangement::Arrangement;
use crate::command::Command;
use crate::error::Result;
use crate::layout::Layout;
use crate::pane_instance::PaneInstance;
use crate::undo_step::UndoStep;

mod adopt;
mod withdraw;

/// Most undo steps kept; the oldest is forgotten first.
pub const MAX_UNDO_STEPS: usize = 64;

/// Most closed panes history keeps alive across all steps. A pane holds at most 64 KiB of
/// state, so this bounds history memory at 16 MiB however large the layout is.
pub const MAX_PARKED_PANES: usize = 256;

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

    /// Whether [`LayoutHistory::undo`] has something to do.
    #[must_use]
    pub fn can_undo(&self) -> bool {
        !self.undo.is_empty()
    }

    /// Whether [`LayoutHistory::redo`] has something to do.
    #[must_use]
    pub fn can_redo(&self) -> bool {
        !self.redo.is_empty()
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
}

#[cfg(test)]
mod tests {
    use protocol::{PaneId, PaneState, PaneTypeId, Params};
    use serde_json::json;

    use super::*;
    use crate::edge::Edge;
    use crate::pane_instance::PaneInstance;
    use crate::placement::Placement;
    use crate::tab_id::TabId;

    fn id(name: &str) -> PaneId {
        PaneId::new(name).unwrap()
    }

    fn instance(name: &str) -> PaneInstance {
        PaneInstance {
            pane_id: id(name),
            type_id: PaneTypeId::new(PaneTypeId::CHAT).unwrap(),
            params: Params::default(),
            state: PaneState::default(),
        }
    }

    fn open(name: &str) -> Command {
        Command::OpenPane {
            tab: TabId(1),
            instance: instance(name),
            placement: None,
        }
    }

    fn add(tab: u64) -> Command {
        Command::AddTab {
            id: TabId(tab),
            title: format!("t{tab}"),
            space: crate::SpaceId::default(),
        }
    }

    fn history() -> LayoutHistory {
        let mut history = LayoutHistory::default();
        for command in [add(1), open("a"), open("b")] {
            history.apply(command).unwrap();
        }
        history
    }

    fn count(
        history: &LayoutHistory,
        tab: u64,
    ) -> Option<usize> {
        history.layout().tab(TabId(tab)).map(crate::tab::Tab::len)
    }

    fn state(value: serde_json::Value) -> PaneState {
        PaneState::new(value).unwrap()
    }

    fn set_state(
        name: &str,
        value: serde_json::Value,
    ) -> Command {
        Command::SetPaneState {
            pane: id(name),
            state: state(value),
        }
    }

    fn move_left_of(
        pane: &str,
        target: &str,
    ) -> Command {
        let placement = Placement {
            target: id(target),
            edge: Edge::Left,
        };
        Command::MovePane {
            pane: id(pane),
            placement,
        }
    }

    fn apply_all(
        history: &mut LayoutHistory,
        commands: Vec<Command>,
    ) {
        for command in commands {
            history.apply(command).unwrap();
        }
    }

    fn fill_tab(
        layout: &mut Layout,
        tab: u64,
    ) {
        layout.apply(add(tab)).unwrap();
        for pane in 0..crate::tab::MAX_PANES_PER_TAB {
            layout
                .apply(open_in(tab, &format!("p{tab}-{pane}")))
                .unwrap();
        }
    }

    fn open_in(
        tab: u64,
        name: &str,
    ) -> Command {
        Command::OpenPane {
            tab: TabId(tab),
            instance: instance(name),
            placement: None,
        }
    }

    fn rename(
        tab: u64,
        title: String,
    ) -> Command {
        Command::RenameTab {
            tab: TabId(tab),
            title,
        }
    }

    fn swap(
        a: &str,
        b: &str,
    ) -> Command {
        Command::SwapPanes { a: id(a), b: id(b) }
    }

    #[test]
    fn undo_and_redo_walk_structural_changes() {
        let mut history = history();
        let full = history.layout().clone();
        assert!(history.undo(), "undo open b");
        assert_eq!(count(&history, 1), Some(1), "one pane");
        assert!(history.can_redo(), "redo available");
        assert!(history.redo(), "redo");
        assert_eq!(history.layout(), &full, "back to full");
        assert!(!history.can_redo(), "nothing left to redo");
    }

    #[test]
    fn a_new_command_clears_redo() {
        let mut history = history();
        assert!(history.undo(), "undo");
        history.apply(open("c")).unwrap();
        assert!(!history.can_redo(), "redo cleared");
    }

    #[test]
    fn refused_commands_leave_history_alone() {
        let mut history = history();
        let before = history.clone();
        assert!(history.apply(open("a")).is_err(), "duplicate");
        assert_eq!(history, before, "unchanged");
    }

    #[test]
    fn typing_into_a_pane_is_not_an_undo_step() {
        let mut history = LayoutHistory::default();
        history.apply(add(1)).unwrap();
        history.apply(open("a")).unwrap();
        let steps = history.undo.len();
        for keystroke in 0..500 {
            let text = format!("typed {keystroke}");
            history
                .apply(set_state("a", json!({ "draft": text })))
                .unwrap();
        }
        assert_eq!(history.undo.len(), steps, "history did not grow");
        assert_eq!(history.parked_total(), 0, "nothing parked");
    }

    #[test]
    fn commands_that_change_nothing_are_not_recorded() {
        let cases = [
            ("focus", Command::FocusPane { pane: id("a") }),
            (
                "activate the active tab",
                Command::ActivateTab { tab: TabId(1) },
            ),
            (
                "move a tab to where it is",
                Command::MoveTab {
                    tab: TabId(1),
                    to_index: 0,
                },
            ),
            (
                "rename to the same title",
                Command::RenameTab {
                    tab: TabId(1),
                    title: "t1".into(),
                },
            ),
        ];
        for (name, command) in cases {
            let mut history = history();
            let steps = history.undo.len();
            history.apply(command).unwrap();
            assert_eq!(history.undo.len(), steps, "{name}");
        }
    }

    #[test]
    fn every_swap_of_panes_is_an_undo_step() {
        let mut history = history();
        let steps = history.undo.len();
        for _ in 0..2 {
            history.apply(swap("a", "b")).unwrap();
        }
        assert_eq!(history.undo.len(), steps + 2, "both swaps");
    }

    #[test]
    fn undoing_a_close_restores_the_pane_with_the_state_it_had_when_it_closed() {
        let mut history = history();
        history
            .apply(set_state("b", json!({"draft": "unsent"})))
            .unwrap();
        history.apply(Command::ClosePane { pane: id("b") }).unwrap();
        assert_eq!(count(&history, 1), Some(1), "closed");
        assert!(history.undo(), "undo the close");
        let restored = history.layout().pane(&id("b")).map(|p| p.state.clone());
        assert_eq!(
            restored,
            Some(state(json!({"draft": "unsent"}))),
            "state came back"
        );
        history.layout().validate().unwrap();
    }

    #[test]
    fn undo_keeps_the_latest_state_of_panes_that_stay_open() {
        let mut history = history();
        history.apply(Command::ClosePane { pane: id("b") }).unwrap();
        history.apply(set_state("a", json!({"scroll": 7}))).unwrap();
        assert!(history.undo(), "undo the close");
        assert_eq!(
            history.layout().pane(&id("a")).map(|p| p.state.clone()),
            Some(state(json!({"scroll": 7}))),
            "work since the close survives"
        );
    }

    #[test]
    fn closing_the_last_pane_of_a_tab_is_one_undo_step() {
        let mut history = LayoutHistory::default();
        for command in [add(1), open("a"), add(2)] {
            history.apply(command).unwrap();
        }
        let before = history.layout().clone();
        history.apply(Command::ClosePane { pane: id("a") }).unwrap();
        assert_eq!(
            history.layout().tabs().len(),
            1,
            "tab closed with its last pane"
        );
        assert!(history.undo(), "one undo");
        assert_eq!(history.layout(), &before, "pane and tab are back");
        assert!(history.redo(), "redo");
        assert!(history.layout().pane(&id("a")).is_none(), "closed again");
    }

    #[test]
    fn a_cross_tab_change_is_one_undo_step() {
        let cases = [
            (
                "detaching",
                vec![],
                Command::DetachPane {
                    pane: id("b"),
                    new_tab: TabId(2),
                    title: "b".into(),
                },
                "b",
            ),
            (
                "moving to another tab",
                vec![add(2)],
                Command::MovePaneToTab {
                    pane: id("a"),
                    to: TabId(2),
                    placement: None,
                },
                "a",
            ),
        ];
        for (name, prelude, command, moved) in cases {
            let mut history = history();
            apply_all(&mut history, prelude);
            let before = history.layout().clone();
            history.apply(command).unwrap();
            assert_eq!(history.layout().tabs().len(), 2, "{name}: two tabs");
            let holder = history.layout().tab_of(&id(moved)).map(crate::tab::Tab::id);
            assert_eq!(holder, Some(TabId(2)), "{name}: pane is in the other tab");
            assert!(history.undo(), "{name}: one undo");
            assert_eq!(history.layout(), &before, "{name}: back");
        }
    }

    #[test]
    fn history_is_bounded_in_steps() {
        let mut history = LayoutHistory::default();
        history.apply(add(1)).unwrap();
        for index in 0..(MAX_UNDO_STEPS + 40) {
            history.apply(rename(1, format!("title {index}"))).unwrap();
        }
        assert_eq!(history.undo.len(), MAX_UNDO_STEPS, "oldest forgotten");
        let mut undone = 0;
        while history.undo() {
            undone += 1;
        }
        assert_eq!(undone, MAX_UNDO_STEPS, "exactly the kept steps");
    }

    #[test]
    fn history_is_bounded_in_closed_panes_it_keeps_alive() {
        let mut layout = Layout::default();
        let tabs = 5_u64;
        for tab in 0..tabs {
            fill_tab(&mut layout, tab);
        }
        let mut history = LayoutHistory::new(layout);
        for tab in 0..tabs {
            history
                .apply(Command::CloseTab { tab: TabId(tab) })
                .unwrap();
        }
        assert!(
            history.parked_total() <= MAX_PARKED_PANES,
            "{}",
            history.parked_total()
        );
        assert_eq!(
            history.undo.len(),
            MAX_PARKED_PANES / crate::tab::MAX_PANES_PER_TAB,
            "oldest closes forgotten"
        );
        assert!(history.undo(), "the newest close is still undoable");
    }

    #[test]
    fn a_step_that_can_no_longer_be_applied_is_dropped_not_half_applied() {
        let mut history = history();
        history.apply(Command::ClosePane { pane: id("b") }).unwrap();
        if let Some(step) = history.undo.back_mut() {
            step.parked.clear();
        }
        let before = history.layout().clone();
        let steps = history.undo.len();
        assert!(!history.undo(), "cannot restore a pane that is gone");
        assert_eq!(history.layout(), &before, "layout untouched");
        assert_eq!(
            history.undo.len(),
            steps - 1,
            "the broken step is discarded"
        );
    }

    #[test]
    fn undo_restores_focus_and_the_active_tab() {
        let mut history = history();
        history.apply(add(2)).unwrap();
        history.apply(Command::FocusPane { pane: id("a") }).unwrap();
        history.apply(move_left_of("b", "a")).unwrap();
        assert!(history.undo(), "undo the move");
        assert_eq!(
            history.layout().showing("").map(crate::tab::Tab::id),
            Some(TabId(1)),
            "tab"
        );
        assert_eq!(
            history.layout().tab(TabId(1)).and_then(|t| t.focused()),
            Some(&id("a")),
            "focus"
        );
    }

    #[test]
    fn nothing_to_undo_or_redo_is_reported() {
        let mut history = LayoutHistory::new(Layout::default());
        assert!(!history.undo() && !history.redo(), "empty");
        assert!(!history.can_undo() && !history.can_redo(), "flags");
    }
}
