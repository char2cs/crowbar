use std::ops::Range;

use editor::LineEdit;
use gpui::{Context, Window};
use gpui_kit::component::input::EditorState;

use super::line_command::LineCommand;
use crate::Access;
use crate::editor_view::EditorView;

impl EditorView {
    pub(in crate::editor_view) fn run_line_command(
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
        let selections = super::selections::of(state);
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

    pub(in crate::editor_view) fn select_all_of(
        &self,
        ranges: &[Range<usize>],
        cx: &mut Context<'_, Self>,
    ) {
        let Some((active, rest)) = ranges.split_last() else {
            return;
        };
        self.state.update(cx, |state, cx| {
            select_with_active(state, active, rest, cx);
        });
    }
}

fn select_with_active(
    state: &mut EditorState,
    active: &Range<usize>,
    rest: &[Range<usize>],
    cx: &mut Context<'_, EditorState>,
) {
    let shown = state.range_to_bounds(active).is_some();
    let scroll = state.scroll_offset();
    state.set_selected_range(active.clone(), cx);
    for range in rest {
        state.add_selection(range.clone(), cx);
    }
    if shown {
        return state.set_scroll_offset(scroll, cx);
    }
    crate::editor_view::reveal::centring::reveal_centered(state, active.start, cx);
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
    fn selecting_several_ranges_leaves_the_last_one_active(cx: &mut TestAppContext) {
        let (view, window) = opened(("ab ab ab", Some(1)), Access::Editable, cx);
        let ranges = [0..2, 3..5, 6..8];
        view.update(window, |view, cx| view.select_all_of(&ranges, cx));
        let active = window.read(|cx| view.read(cx).state.read(cx).selected_range());
        assert_eq!(active, 6..8);
    }
}
