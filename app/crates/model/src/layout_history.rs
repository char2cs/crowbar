use crate::command::Command;
use crate::error::Result;
use crate::layout::Layout;

const MAX_UNDO: usize = 64;

/// A [`Layout`] with undo and redo of structural commands.
///
/// Undo restores the arrangement (tabs, splits, which pane sits where) but keeps the latest
/// state of every pane that still exists, so undoing a close never rolls back work.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct LayoutHistory {
    current: Layout,
    undo: Vec<Layout>,
    redo: Vec<Layout>,
}

impl LayoutHistory {
    /// Starts a history at `layout`.
    #[must_use]
    pub fn new(layout: Layout) -> Self {
        Self {
            current: layout,
            undo: Vec::new(),
            redo: Vec::new(),
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

    /// Applies a command; a structural one becomes undoable and clears redo.
    ///
    /// # Errors
    /// The [`Error`](crate::Error) that refused the command; history is unchanged then.
    pub fn apply(&mut self, command: Command) -> Result<()> {
        let structural = command.is_structural();
        let before = self.current.clone();
        self.current.apply(command)?;
        if structural {
            self.record(before);
        }
        Ok(())
    }

    fn record(&mut self, before: Layout) {
        self.undo.push(before);
        if self.undo.len() > MAX_UNDO {
            self.undo.remove(0);
        }
        self.redo.clear();
    }

    /// Steps back one structural change. Returns whether anything changed.
    pub fn undo(&mut self) -> bool {
        let Some(mut previous) = self.undo.pop() else {
            return false;
        };
        previous.adopt_states(&self.current);
        self.redo
            .push(std::mem::replace(&mut self.current, previous));
        true
    }

    /// Steps forward again. Returns whether anything changed.
    pub fn redo(&mut self) -> bool {
        let Some(mut next) = self.redo.pop() else {
            return false;
        };
        next.adopt_states(&self.current);
        self.undo.push(std::mem::replace(&mut self.current, next));
        true
    }
}

#[cfg(test)]
#[path = "layout_history_tests.rs"]
mod tests;
