//! Undo and redo snapshots of an editable text.

mod edit;

pub(crate) use edit::Edit;

use std::collections::VecDeque;

use crate::control::editor::text_buffer::TextBuffer;

const DEPTH: usize = 200;
const BYTE_BUDGET: usize = 4 * 1024 * 1024;

#[derive(Debug, Default)]
pub(crate) struct TextHistory {
    undo: VecDeque<TextBuffer>,
    redo: Vec<TextBuffer>,
    typing: bool,
}

impl TextHistory {
    pub(crate) fn record(
        &mut self,
        before: &TextBuffer,
        edit: Edit,
    ) {
        let continues = edit == Edit::Typing && self.typing;
        self.typing = edit == Edit::Typing;
        self.redo.clear();
        if continues {
            return;
        }
        self.undo.push_back(before.clone());
        self.trim();
    }

    fn trim(&mut self) {
        let mut bytes = self.bytes();
        while let Some(freed) = self.evict_oldest(bytes) {
            bytes -= freed;
        }
    }

    fn evict_oldest(
        &mut self,
        bytes: usize,
    ) -> Option<usize> {
        let too_large = self.undo.len() > DEPTH || bytes > BYTE_BUDGET;
        if self.undo.len() < 2 || !too_large {
            return None;
        }
        self.undo.pop_front().map(|oldest| oldest.text().len())
    }

    fn bytes(&self) -> usize {
        self.undo.iter().map(|buffer| buffer.text().len()).sum()
    }

    pub(crate) fn undo(
        &mut self,
        current: &TextBuffer,
    ) -> Option<TextBuffer> {
        let previous = self.undo.pop_back()?;
        self.redo.push(current.clone());
        self.typing = false;
        Some(previous)
    }

    pub(crate) fn redo(
        &mut self,
        current: &TextBuffer,
    ) -> Option<TextBuffer> {
        let next = self.redo.pop()?;
        self.undo.push_back(current.clone());
        self.typing = false;
        Some(next)
    }

    pub(crate) fn break_typing(&mut self) {
        self.typing = false;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn buffer(text: &str) -> TextBuffer {
        TextBuffer::with_text(text)
    }

    fn snapshots_after(edits: &[Edit]) -> usize {
        let mut history = TextHistory::default();
        for &edit in edits {
            history.record(&buffer("x"), edit);
        }
        history.undo.len()
    }

    #[test]
    fn typing_coalesces_and_other_edits_break_the_run() {
        const CASES: &[(&[Edit], usize)] = &[
            (&[Edit::Typing, Edit::Typing, Edit::Typing], 1),
            (&[Edit::Typing, Edit::Other, Edit::Typing], 3),
            (&[Edit::Other, Edit::Other], 2),
        ];
        for &(edits, snapshots) in CASES {
            assert_eq!(snapshots_after(edits), snapshots, "{edits:?}");
        }
    }

    #[test]
    fn undo_and_redo_swap_snapshots_and_a_new_edit_drops_the_redo_branch() {
        let mut history = TextHistory::default();
        history.record(&buffer("a"), Edit::Other);
        let undone = history.undo(&buffer("ab"));
        assert_eq!(undone.as_ref().map(TextBuffer::text), Some("a"));
        let redone = history.redo(&buffer("a"));
        assert_eq!(redone.as_ref().map(TextBuffer::text), Some("ab"));
        history.undo(&buffer("ab"));
        history.record(&buffer("a"), Edit::Other);
        assert!(history.redo(&buffer("ac")).is_none());
    }

    #[test]
    fn breaking_the_run_starts_a_new_snapshot() {
        let mut history = TextHistory::default();
        history.record(&buffer("a"), Edit::Typing);
        history.break_typing();
        history.record(&buffer("ab"), Edit::Typing);
        assert_eq!(history.undo.len(), 2);
    }

    #[test]
    fn large_snapshots_are_capped_by_size_but_the_latest_is_kept() {
        let mut history = TextHistory::default();
        let large = buffer(&"x".repeat(BYTE_BUDGET / 4 + 1));
        for _ in 0..10 {
            history.record(&large, Edit::Other);
        }
        assert!(history.undo.len() <= 4 && !history.undo.is_empty());
    }

    #[test]
    fn the_history_is_capped() {
        let mut history = TextHistory::default();
        for _ in 0..DEPTH + 5 {
            history.record(&buffer("x"), Edit::Other);
        }
        assert_eq!(history.undo.len(), DEPTH);
    }
}
