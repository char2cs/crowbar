use std::ops::Range;

use gpui::{Context, Entity, SharedString, Window};
use ropey::Rope;

use super::{
    InputBaseState, InputModeKind, RopeExt as _, TextModel, TextModelEdit,
    cursor::CursorSelection, mode::HighlighterUpdate,
};

impl<M: InputModeKind> InputBaseState<M> {
    /// Show `model`'s text and share it from now on: edits made here reach
    /// the model, and edits other states make to it are followed here, each
    /// state keeping its own selections, scroll and folds. Undo and redo act
    /// on the model's history, so they undo the document's last edit
    /// whichever state made it.
    pub fn share_text(
        &mut self,
        model: Entity<TextModel>,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        let text = SharedString::from(model.read(cx).text().to_string());
        self.text_model = None;
        self.set_value(text, window, cx);
        self.undo_manager = model.read(cx).history();
        let edits = cx.subscribe_in(&model, window, Self::follow_text_model);
        self._subscriptions.push(edits);
        self.text_model = Some(model);
    }

    /// The shared document this state shows, if it was given one.
    pub fn text_model(&self) -> Option<&Entity<TextModel>> {
        self.text_model.as_ref()
    }

    pub(super) fn publish_edit(
        &mut self,
        range: &Range<usize>,
        new_text: &str,
        cx: &mut Context<Self>,
    ) {
        let Some(model) = self.text_model.clone() else {
            return;
        };
        let edit = TextModelEdit {
            origin: cx.entity_id(),
            range: range.clone(),
            new_text: new_text.to_owned(),
            text: self.text.clone(),
        };
        model.update(cx, |model, cx| model.push(edit, cx));
    }

    fn follow_text_model(
        &mut self,
        _: &Entity<TextModel>,
        edit: &TextModelEdit,
        window: &mut Window,
        cx: &mut Context<Self>,
    ) {
        if edit.origin == cx.entity_id() {
            return;
        }
        let old_text = std::mem::replace(&mut self.text, edit.text.clone());
        let (range, new_text) = (&edit.range, edit.new_text.as_str());
        self.document_revision = self.document_revision.wrapping_add(1);
        self.mode.adjust_auto_closed_pair(range, new_text.len());
        M::adjust_annotations(self, range, new_text.len());
        self.display_map
            .adjust_folds_for_edit(&old_text, range, new_text);
        self.display_map
            .on_text_changed(&self.text, range, &Rope::from(new_text), cx);
        let input_edit = super::mode::replacement_input_edit(&old_text, &self.text, range, new_text);
        self.mode.update_highlighter(
            HighlighterUpdate {
                selected_range: range,
                old_text: &old_text,
                new_text: &self.text,
                change_text: new_text,
                force: true,
                input_edit: Some(input_edit),
            },
            window,
            cx,
        );
        self.update_fold_candidates_incremental(range, new_text);
        if let Some(diagnostics) = self.mode.diagnostics_mut() {
            diagnostics.reset(&self.text)
        }
        M::refresh_language_features(self, window, cx);
        self.update_search(cx);
        self.follow_selections(&old_text, range, new_text.len());
        self.ime_marked_range = None;
        if self.is_multi_line() {
            self.mode.update_auto_grow(&self.display_map);
        }
        cx.notify();
    }

    /// Another state's edit moves this state's selections as Monaco moves the
    /// cursors of every other editor on a model: a replacement of the whole
    /// text keeps them at their line and column, any other edit shifts the
    /// offsets after it.
    fn follow_selections(&mut self, old_text: &Rope, range: &Range<usize>, new_len: usize) {
        let whole = range.start == 0 && range.end >= old_text.len();
        let lines = self.text.lines_len();
        let followed: Vec<CursorSelection> = self
            .selections
            .iter()
            .map(|selection| {
                let mut selection = *selection;
                let (start, end) = if whole {
                    (
                        self.offset_within(&old_text.offset_to_position(selection.start), lines),
                        self.offset_within(&old_text.offset_to_position(selection.end), lines),
                    )
                } else {
                    (
                        shifted(selection.start, range, new_len),
                        shifted(selection.end, range, new_len),
                    )
                };
                selection.start = start;
                selection.end = end;
                selection
            })
            .collect();
        self.restore_selections(Some(followed));
    }
}

fn shifted(offset: usize, range: &Range<usize>, new_len: usize) -> usize {
    if offset < range.start {
        return offset;
    }
    if offset >= range.end {
        return offset - range.len() + new_len;
    }
    range.start + (offset - range.start).min(new_len)
}
