use gpui::Context;
use gpui_kit::component::input::{FoldRange, RopeExt as _};

use super::EditorView;
use super::fold_command::FoldCommand;

impl EditorView {
    pub(super) fn run_fold_command(
        &mut self,
        command: FoldCommand,
        cx: &mut Context<'_, Self>,
    ) {
        let starts = self.fold_starts(command, cx);
        let folded = matches!(command, FoldCommand::Fold | FoldCommand::FoldAll);
        for start in starts {
            self.state
                .update(cx, |state, cx| state.set_folded(start, folded, cx));
        }
    }

    fn fold_starts(
        &self,
        command: FoldCommand,
        cx: &Context<'_, Self>,
    ) -> Vec<usize> {
        let state = self.state.read(cx);
        let map = state.display_map();
        let all = |ranges: &[FoldRange]| ranges.iter().map(|range| range.start_line).collect();
        let caret_lines: Vec<usize> = state
            .selection_ranges()
            .iter()
            .map(|range| state.text().offset_to_point(range.start).row)
            .collect();
        match command {
            FoldCommand::FoldAll => all(state.fold_candidates()),
            FoldCommand::UnfoldAll => all(map.folded_ranges()),
            FoldCommand::Fold => innermost(state.fold_candidates(), &caret_lines, |start| {
                !map.is_folded_at(start)
            }),
            FoldCommand::Unfold => innermost(map.folded_ranges(), &caret_lines, |_| true),
        }
    }
}

fn innermost(
    ranges: &[FoldRange],
    lines: &[usize],
    eligible: impl Fn(usize) -> bool,
) -> Vec<usize> {
    lines
        .iter()
        .filter_map(|line| {
            ranges
                .iter()
                .filter(|range| range.start_line <= *line && *line <= range.end_line)
                .filter(|range| eligible(range.start_line))
                .map(|range| range.start_line)
                .max()
        })
        .collect()
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::Access;
    use crate::editor_view::test_support::{opened, press};

    #[gpui::test]
    fn the_fold_chords_fold_the_caret_s_block_and_open_it_again(cx: &mut TestAppContext) {
        let text = "function f() {\n  if (a) {\n    b();\n  }\n}\n";
        let cases: [(&str, &[usize]); 4] = [
            ("@editorview::Fold", &[1]),
            ("@editorview::Fold @editorview::Fold", &[0, 1]),
            ("@editorview::Fold @editorview::Unfold", &[]),
            ("@editorview::FoldAll", &[0, 1]),
        ];
        for (keys, expected) in cases {
            let (view, window) = opened((text, Some(3)), Access::Editable, cx);
            window.run_until_parked();
            press(keys, window);
            let folded = window.read(|cx| folded_lines(view.read(cx).state.read(cx)));
            assert_eq!(folded, expected, "{keys}");
        }
    }

    fn folded_lines(state: &gpui_kit::component::input::EditorState) -> Vec<usize> {
        let ranges = state.display_map().folded_ranges();
        ranges.iter().map(|range| range.start_line).collect()
    }
}
