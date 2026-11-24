use std::ops::Range;

use gpui::{Context, EntityId, EventEmitter};
use ropey::Rope;

use super::undo_manager::UndoManager;

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
