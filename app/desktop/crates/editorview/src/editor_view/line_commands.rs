use editor::LineEdit;
use gpui::{Context, Window};

use super::EditorView;
use super::line_command::LineCommand;
use crate::Access;

impl EditorView {
    pub(super) fn run_line_command(
        &mut self,
        command: LineCommand,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        if self.access == Access::ReadOnly {
            return;
        }
        let state = self.state.read(cx);
        let text = state.value();
        let selections = state.selection_ranges();
        let edit = match command {
            LineCommand::Move(toward) => LineEdit::moved(&text, &selections, toward),
            LineCommand::Copy(toward) => LineEdit::copied(&text, &selections, toward),
            LineCommand::Delete => LineEdit::deleted(&text, &selections),
        };
        let Some(edit) = edit else {
            return;
        };
        self.state.update(cx, |state, cx| {
            state.set_selected_range(edit.range(), cx);
            state.replace(edit.text().to_owned(), window, cx);
        });
        self.select_all_of(edit.selections(), cx);
    }

    pub(super) fn select_all_of(
        &self,
        ranges: &[std::ops::Range<usize>],
        cx: &mut Context<'_, Self>,
    ) {
        let Some((first, rest)) = ranges.split_first() else {
            return;
        };
        self.state
            .update(cx, |state, cx| state.set_selected_range(first.clone(), cx));
        for range in rest {
            self.state
                .update(cx, |state, cx| state.add_selection(range.clone(), cx));
        }
    }
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::editor_view::test_support::{opened, press};
    use crate::{Access, EditorView};

    #[gpui::test]
    fn the_line_chords_move_copy_and_delete_the_caret_s_line_undoably(cx: &mut TestAppContext) {
        let cases = [
            (Access::Editable, "alt-down", "b\na\nc"),
            (Access::Editable, "alt-down alt-down alt-up", "b\na\nc"),
            (Access::Editable, "alt-up", "a\nb\nc"),
            (
                Access::Editable,
                "@editorview::CopyLinesDown x",
                "a\nxa\nb\nc",
            ),
            (
                Access::Editable,
                "@editorview::CopyLinesUp x",
                "xa\na\nb\nc",
            ),
            (Access::Editable, "@editorview::DeleteLines", "b\nc"),
            (Access::Editable, "alt-down @input::Undo", "a\nb\nc"),
            (
                Access::Editable,
                "alt-down @input::Undo @input::Redo",
                "b\na\nc",
            ),
            (
                Access::ReadOnly,
                "alt-down @editorview::DeleteLines",
                "a\nb\nc",
            ),
        ];
        for (access, keys, expected) in cases {
            let (view, window) = opened(("a\nb\nc", Some(1)), access, cx);
            press(keys, window);
            assert_eq!(view.read_with(window, EditorView::text), expected, "{keys}");
        }
    }

    #[gpui::test]
    fn selecting_several_ranges_leaves_every_one_selected(cx: &mut TestAppContext) {
        let (view, window) = opened(("ab ab ab", Some(1)), Access::Editable, cx);
        let ranges = [0..2, 3..5, 6..8];
        view.update(window, |view, cx| view.select_all_of(&ranges, cx));
        let selected = window.read(|cx| view.read(cx).state.read(cx).selection_ranges());
        assert_eq!(selected, ranges);
    }
}
