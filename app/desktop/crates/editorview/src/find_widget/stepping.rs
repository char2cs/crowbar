use std::ops::Range;

use editor::Vertical;
use gpui::Context;
use gpui_kit::component::input::{EditorState, RopeExt as _};

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
    let row = state.text().offset_to_point(range.start).row;
    let shown = state
        .visible_row_range()
        .is_some_and(|rows| rows.contains(&row));
    if !shown {
        state.reveal_centered(range.start, cx);
    }
}

#[cfg(test)]
mod tests {
    use editor::Vertical;
    use gpui::TestAppContext;

    use crate::Access;
    use crate::editor_view::test_support::{find_of, on_find, opened, selected_of};
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
