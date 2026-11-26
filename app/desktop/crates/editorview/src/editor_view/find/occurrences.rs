use std::ops::Range;

use editor::{FindOption, FindOptions, FindQuery};
use gpui::Context;

use crate::editor_view::EditorView;
use crate::editor_view::find::occurrence_session::OccurrenceSession;

const WORD_SEARCH: &[FindOption] = &[FindOption::MatchCase, FindOption::WholeWord];

impl EditorView {
    pub(in crate::editor_view) fn add_next_occurrence(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let state = self.state.read(cx);
        let text = state.value();
        let active = state.selected_range();
        if active.is_empty() {
            return self.start_from_word(active.start, cx);
        }
        let (query, mut selections) = self.session_at(&text, &active);
        let Ok(found) = query.matches(&text) else {
            return;
        };
        let unselected = |range: &&Range<usize>| !selections.contains(range);
        let after = found
            .iter()
            .filter(unselected)
            .find(|range| range.start >= active.end);
        let next = after.or_else(|| found.iter().find(unselected)).cloned();
        let Some(next) = next else {
            return;
        };
        selections.push(next);
        self.select_all_of(&selections, cx);
        self.occurrences = Some(OccurrenceSession { query, selections });
    }

    pub(in crate::editor_view) fn select_all_occurrences(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) {
        let state = self.state.read(cx);
        let text = state.value();
        let active = state.selected_range();
        let word = editor::word_at(&text, active.start);
        let (query, around) = match (active.is_empty(), word) {
            (true, Some(word)) => (word_query(&text, &word), word),
            (true, None) => return,
            (false, _) => (selected_query(&text, &active), active),
        };
        let Ok(found) = query.matches(&text) else {
            return;
        };
        let primary = found.iter().position(|range| *range == around).unwrap_or(0);
        let ordered: Vec<Range<usize>> = found
            .iter()
            .skip(primary)
            .chain(found.iter().take(primary))
            .cloned()
            .collect();
        self.select_all_of(&ordered, cx);
    }

    fn start_from_word(
        &mut self,
        caret: usize,
        cx: &mut Context<'_, Self>,
    ) {
        let text = self.state.read(cx).value();
        let Some(word) = editor::word_at(&text, caret) else {
            return;
        };
        let query = word_query(&text, &word);
        self.state
            .update(cx, |state, cx| state.set_selected_range(word.clone(), cx));
        let selections = vec![word];
        self.occurrences = Some(OccurrenceSession { query, selections });
    }

    fn session_at(
        &self,
        text: &str,
        active: &Range<usize>,
    ) -> (FindQuery, Vec<Range<usize>>) {
        let ongoing = self
            .occurrences
            .as_ref()
            .filter(|session| session.selections.last() == Some(active));
        ongoing.map_or_else(
            || (selected_query(text, active), vec![active.clone()]),
            |session| (session.query.clone(), session.selections.clone()),
        )
    }
}

fn word_query(
    text: &str,
    word: &Range<usize>,
) -> FindQuery {
    let word = text.get(word.clone()).unwrap_or("");
    FindQuery::new(word, FindOptions::of(WORD_SEARCH))
}

fn selected_query(
    text: &str,
    selected: &Range<usize>,
) -> FindQuery {
    let selected = text.get(selected.clone()).unwrap_or("");
    FindQuery::new(selected, FindOptions::default())
}

#[cfg(test)]
mod tests {
    use gpui::TestAppContext;

    use crate::editor_view::test_support::{opened, press};
    use crate::{Access, EditorView};

    #[gpui::test]
    fn ctrl_d_and_ctrl_shift_l_select_the_caret_word_s_occurrences(cx: &mut TestAppContext) {
        let text = "total Total totals total";
        let cases = [
            ("@editorview::AddNextOccurrence", "y Total totals total"),
            (
                "@editorview::AddNextOccurrence @editorview::AddNextOccurrence",
                "y Total totals y",
            ),
            (
                "@editorview::AddNextOccurrence @editorview::AddNextOccurrence @editorview::AddNextOccurrence",
                "y Total totals y",
            ),
            ("@editorview::SelectAllOccurrences", "y Total totals y"),
            (
                "shift-right shift-right @editorview::AddNextOccurrence",
                "ytal ytal totals total",
            ),
        ];
        for (keys, expected) in cases {
            let (view, window) = opened((text, Some(1)), Access::Editable, cx);
            press(keys, window);
            window.simulate_input("y");
            assert_eq!(view.read_with(window, EditorView::text), expected, "{keys}");
        }
    }
}
