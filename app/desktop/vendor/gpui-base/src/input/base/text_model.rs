use std::ops::Range;

use gpui::{Context, EntityId, EventEmitter};
use ropey::Rope;

use super::undo_manager::{EditIntent, UndoManager};
use crate::input::change::Change;

/// One document's text and undo history, shared by every editor state that
/// shows it.
///
/// A state made with [`super::InputBaseState::share_text`] applies its edits
/// here, and every other state over the same model follows them while
/// keeping its own selections, scroll and folds, as several Monaco editors
/// share one text model.
pub struct TextModel {
    text: Rope,
    revision: u64,
    history: UndoManager,
}

/// One replacement a state applied to a [`TextModel`], in the coordinates of
/// the text before it.
#[derive(Clone)]
pub struct TextModelEdit {
    pub(crate) origin: EntityId,
    pub(crate) range: Range<usize>,
    pub(crate) new_text: String,
    pub(crate) text: Rope,
}

impl EventEmitter<TextModelEdit> for TextModel {}

impl TextModel {
    /// A model holding `text`, with an empty history.
    pub fn new(text: &str) -> Self {
        Self {
            text: Rope::from(text),
            revision: 0,
            history: UndoManager::new(),
        }
    }

    /// The text as it stands after the last edit.
    pub fn text(&self) -> &Rope {
        &self.text
    }

    /// Advances with every edit, whichever state made it.
    pub fn revision(&self) -> u64 {
        self.revision
    }

    /// Replace the whole text as one undo step from outside any editor,
    /// such as the file read from disk: every state over the model keeps its
    /// selections at their line and column. The same text changes nothing.
    pub fn replace_all(&mut self, text: &str, cx: &mut Context<Self>) {
        if self.text == text {
            return;
        }
        let range = 0..self.text.len();
        let change = Change::new(range.clone(), &self.text.to_string(), 0..text.len(), text);
        self.history.record_transaction(change, EditIntent::Atomic);
        let edit = TextModelEdit {
            origin: cx.entity_id(),
            range,
            new_text: text.to_owned(),
            text: Rope::from(text),
        };
        self.push(edit, cx);
    }

    pub(crate) fn history(&self) -> UndoManager {
        self.history.clone()
    }

    pub(crate) fn push(&mut self, edit: TextModelEdit, cx: &mut Context<Self>) {
        self.text = edit.text.clone();
        self.revision = self.revision.wrapping_add(1);
        cx.emit(edit);
        cx.notify();
    }
}
