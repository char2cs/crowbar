use std::ops::Range;

use editor::Vertical;
use gpui::Context;
use gpui_kit::component::input::EditorState;

use super::FindWidget;

impl FindWidget {
    pub(crate) fn step(
        &mut self,
        toward: Vertical,
        cx: &mut Context<'_, Self>,
    ) {
        let selected = self.editor.read(cx).selected_range();
        let target = match toward {
            Vertical::Down => self.after(selected.end.max(selected.start)),
            Vertical::Up => self.before(selected.start),
        };
        self.select(target, cx);
    }

    pub(super) fn typed(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        self.research(cx);
        let selected = self.editor.read(cx).selected_range();
        let target = self.after(selected.start);
        self.select(target, cx);
    }

    fn after(
        &self,
        offset: usize,
    ) -> Option<usize> {
        let next = self.matches.iter().position(|range| range.start >= offset);
        next.or_else(|| (!self.matches.is_empty()).then_some(0))
    }

    fn before(
        &self,
        offset: usize,
    ) -> Option<usize> {
        let previous = self.matches.iter().rposition(|range| range.end <= offset);
        previous.or_else(|| self.matches.len().checked_sub(1))
    }

    fn select(
        &mut self,
        index: Option<usize>,
        cx: &mut Context<'_, Self>,
    ) {
        let Some(index) = index else {
            return;
        };
        let Some(range) = self.matches.get(index).cloned() else {
            return;
        };
        self.current = Some(index);
        self.editor.update(cx, |state, cx| {
            state.set_selected_range(range.clone(), cx);
            reveal_if_hidden(state, &range, cx);
        });
        self.paint(cx);
    }
}

fn reveal_if_hidden(
    state: &mut EditorState,
    range: &Range<usize>,
    cx: &mut Context<'_, EditorState>,
) {
    if state.shows_line_whole(range.start) == Some(true) {
        return;
    }
    state.reveal_centered(range.start, cx);
}

#[cfg(test)]
mod tests {
    use editor::Vertical;
    use gpui::TestAppContext;

    use crate::Access;
    use crate::editor_view::test_support::{
        body_of, find_of, line_height_of, on_find, opened, scroll_top_of, selected_of,
    };
    use crate::find_widget::FindWidget;
    use crate::reveal::Reveal;

    type Case = (&'static [Vertical], std::ops::Range<usize>);

    const CASES: &[Case] = &[
        (&[Vertical::Down], 0..1),
        (&[Vertical::Down, Vertical::Down], 4..5),
        (
            &[
                Vertical::Down,
                Vertical::Down,
                Vertical::Down,
                Vertical::Down,
            ],
            0..1,
        ),
        (&[Vertical::Up], 8..9),
        (&[Vertical::Down, Vertical::Up], 8..9),
    ];

    #[gpui::test]
    fn a_step_selects_the_next_or_previous_match_and_wraps_at_the_ends(cx: &mut TestAppContext) {
        for (steps, expected) in CASES {
            let (view, window) = opened(("a b a b a", Some(1)), Access::Editable, cx);
            let find = find_of(&view, window);
            walk(&find, steps, window);
            assert_eq!(selected_of(&view, window), *expected, "{steps:?}");
        }
    }

    const LINES: usize = 200;

    fn viewport(cx: &mut TestAppContext) -> (f32, f32) {
        let text = "line\n".repeat(LINES);
        let (view, window) = opened((&text, Some(1)), Access::Editable, cx);
        searched(&view, "", window);
        let body = body_of(window).map_or(0.0, |body| f32::from(body.size.height));
        let line = line_height_of(&view, window);
        (body, line.map_or(1.0, f32::from))
    }

    fn searched(
        view: &gpui::Entity<crate::EditorView>,
        query: &'static str,
        window: &mut gpui::VisualTestContext,
    ) {
        window.run_until_parked();
        let find = find_of(view, window);
        on_find(&find, window, |find, window, cx| {
            find.open(Reveal::Find, window, cx);
            find.find_input
                .update(cx, |input, cx| input.set_text(query, cx));
            find.typed(cx);
        });
        window.run_until_parked();
    }

    #[gpui::test]
    fn a_match_shown_whole_stays_put_and_one_cut_by_the_fold_is_centred(cx: &mut TestAppContext) {
        let (body, line) = viewport(cx);
        let rows = (0_u16..u16::MAX)
            .take_while(|row| f32::from(row + 1) * line <= body)
            .count();
        let whole = f32::from(u16::try_from(rows).unwrap_or(0));
        let centred = whole * line - (body - line) / 2.0;
        for (word, top) in [("whole", 0.0), ("cut", -centred)] {
            let mut lines = vec!["line"; LINES];
            lines[rows - 1] = "whole";
            lines[rows] = "cut";
            let (view, window) = opened((&lines.join("\n"), Some(1)), Access::Editable, cx);
            searched(&view, word, window);
            let shown = f32::from(scroll_top_of(&view, window));
            assert!((shown - top).abs() < line, "{word}: top {shown}, not {top}");
        }
    }

    fn walk(
        find: &gpui::Entity<FindWidget>,
        steps: &'static [Vertical],
        window: &mut gpui::VisualTestContext,
    ) {
        on_find(find, window, |find, window, cx| {
            find.open(Reveal::Find, window, cx);
            step_all(find, steps, cx);
        });
    }

    fn step_all(
        find: &mut FindWidget,
        steps: &[Vertical],
        cx: &mut gpui::Context<'_, FindWidget>,
    ) {
        for toward in steps {
            find.step(*toward, cx);
        }
    }
}
