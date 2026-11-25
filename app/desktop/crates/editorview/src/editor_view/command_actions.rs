use editor::Vertical;
use gpui::{Action, App, Context, Div, InteractiveElement as _, Window};

use super::EditorView;
use super::fold_command::FoldCommand;
use super::line_command::LineCommand;
use crate::keys::{
    AddNextOccurrence, CopyLinesDown, CopyLinesUp, DeleteLines, Fold, FoldAll, MoveLinesDown,
    MoveLinesUp, SelectAllOccurrences, Unfold, UnfoldAll,
};

impl EditorView {
    pub(super) fn command_actions(
        element: Div,
        cx: &mut Context<'_, Self>,
    ) -> Div {
        element
            .on_action(line::<MoveLinesUp>(LineCommand::Move(Vertical::Up), cx))
            .on_action(line::<MoveLinesDown>(LineCommand::Move(Vertical::Down), cx))
            .on_action(line::<CopyLinesUp>(LineCommand::Copy(Vertical::Up), cx))
            .on_action(line::<CopyLinesDown>(LineCommand::Copy(Vertical::Down), cx))
            .on_action(line::<DeleteLines>(LineCommand::Delete, cx))
            .on_action(fold::<Fold>(FoldCommand::Fold, cx))
            .on_action(fold::<Unfold>(FoldCommand::Unfold, cx))
            .on_action(fold::<FoldAll>(FoldCommand::FoldAll, cx))
            .on_action(fold::<UnfoldAll>(FoldCommand::UnfoldAll, cx))
            .on_action(cx.listener(|view, _: &AddNextOccurrence, _, cx| {
                view.add_next_occurrence(cx);
            }))
            .on_action(cx.listener(|view, _: &SelectAllOccurrences, _, cx| {
                view.select_all_occurrences(cx);
            }))
    }
}

fn line<A: Action>(
    command: LineCommand,
    cx: &mut Context<'_, EditorView>,
) -> impl Fn(&A, &mut Window, &mut App) + 'static {
    cx.listener(move |view: &mut EditorView, _: &A, window, cx| {
        view.run_line_command(command, window, cx);
    })
}

fn fold<A: Action>(
    command: FoldCommand,
    cx: &mut Context<'_, EditorView>,
) -> impl Fn(&A, &mut Window, &mut App) + 'static {
    cx.listener(move |view: &mut EditorView, _: &A, _, cx| view.run_fold_command(command, cx))
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Access;
    use crate::editor_view::test_support::{opened, press};

    const TEXT: &str = "ab x ab\nab";

    #[gpui::test]
    fn the_occurrence_chords_select_the_caret_word_s_matches(cx: &mut TestAppContext) {
        let cases = [
            ("@editorview::AddNextOccurrence", &[(0, 2)][..]),
            (
                "@editorview::AddNextOccurrence @editorview::AddNextOccurrence",
                &[(0, 2), (5, 7)],
            ),
            (
                "@editorview::SelectAllOccurrences",
                &[(0, 2), (5, 7), (8, 10)],
            ),
        ];
        for (keys, expected) in cases {
            let (view, window) = opened((TEXT, Some(1)), Access::Editable, cx);
            press(keys, window);
            let selected = window.read(|cx| view.read(cx).state.read(cx).selection_ranges());
            let expected: Vec<_> = expected.iter().map(|(start, end)| *start..*end).collect();
            assert_eq!(selected, expected, "{keys}");
        }
    }
}
