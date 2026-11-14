use std::collections::{BTreeMap, VecDeque};

use crate::arrangement::Arrangement;
use crate::command::Command;
use crate::error::Result;
use crate::layout::Layout;
use crate::pane_instance::PaneInstance;
use crate::undo_step::UndoStep;

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
    pub fn apply(&mut self, command: Command) -> Result<()> {
        let before = command.is_structural().then(|| self.current.arrangement());
        let removed = self.current.execute(command)?;
        if let Some(before) = before {
            self.record(before, removed);
        }
        Ok(())
    }

    fn record(&mut self, before: Arrangement, removed: Vec<PaneInstance>) {
        if removed.is_empty() && self.current.arrangement() == before {
            return;
        }
        let parked: BTreeMap<_, _> = removed
            .into_iter()
            .map(|instance| (instance.pane_id.clone(), instance))
            .collect();
        self.undo.push_back(UndoStep {
            arrangement: before,
            parked,
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
#[path = "layout_history_tests.rs"]
mod tests;
