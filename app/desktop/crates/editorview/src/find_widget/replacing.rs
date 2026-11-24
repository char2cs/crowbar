use std::ops::Range;

use editor::Vertical;
use gpui::{Context, Window};

use super::FindWidget;

impl FindWidget {
    pub(crate) fn replace_one(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let selected = self.editor.read(cx).selected_range();
        let on_match = self.matches.contains(&selected);
        if !on_match {
            return self.step(Vertical::Down, cx);
        }
        let Some(replacement) = self.replacement_of(&selected, cx) else {
            return;
        };
        self.editor.update(cx, |state, cx| {
            state.replace(replacement, window, cx);
        });
        self.research(cx);
        self.step(Vertical::Down, cx);
    }

    pub(crate) fn replace_all(
        &mut self,
        window: &mut Window,
        cx: &mut Context<'_, Self>,
    ) {
        let (Some(first), Some(last)) = (self.matches.first(), self.matches.last()) else {
            return;
        };
        let span = first.start..last.end;
        let Some(text) = self.replaced_span(&span, cx) else {
            return;
        };
        self.editor.update(cx, |state, cx| {
            state.set_selected_range(span, cx);
            state.replace(text, window, cx);
        });
        self.research(cx);
    }

    fn replacement_of(
        &self,
        found: &Range<usize>,
        cx: &Context<'_, Self>,
    ) -> Option<String> {
        let text = self.editor.read(cx).value();
        let with = self.replace_input.read(cx).text();
        let replaced = self.query(cx).replacement(&text, found, with);
        replaced
            .map_err(|error| tracing::warn!(%error, "replace: the query does not parse"))
            .ok()
    }

    fn replaced_span(
        &self,
        span: &Range<usize>,
        cx: &Context<'_, Self>,
    ) -> Option<String> {
        let text = self.editor.read(cx).value();
        let ends = std::iter::once(span.start).chain(self.matches.iter().map(|found| found.end));
        let pieces = self.matches.iter().zip(ends).map(|(found, from)| {
            let between = text.get(from..found.start)?;
            let replaced = self.replacement_of(found, cx)?;
            Some(format!("{between}{replaced}"))
        });
        pieces.collect()
    }
}

#[cfg(test)]
mod tests {
    use editor::FindOption;
    use gpui::TestAppContext;

    use crate::editor_view::test_support::{find_of, on_find, opened};
    use crate::find_widget::FindWidget;
    use crate::reveal::Reveal;
    use crate::{Access, EditorView};

    const TEXT: &str = "ab x ab";
    const CASES: &[(&str, &str, &str)] = &[
        ("ab", "$2$1", "$2$1 x $2$1"),
        ("(a)(b)", "$2$1", "ba x ba"),
        ("(a)(b)", "[$&]", "[ab] x [ab]"),
    ];

    #[gpui::test]
    fn replace_all_fills_each_match_from_the_query_s_groups(cx: &mut TestAppContext) {
        for (query, with, expected) in CASES {
            let (view, window) = opened((TEXT, Some(1)), Access::Editable, cx);
            let find = find_of(&view, window);
            fill(&find, (query, with), window);
            on_find(&find, window, FindWidget::replace_all);
            let text = view.read_with(window, EditorView::text);
            assert_eq!(text, *expected, "{query} {with}");
        }
    }

    fn regex_when_grouped(
        find: &mut FindWidget,
        query: &str,
        cx: &mut gpui::Context<'_, FindWidget>,
    ) {
        if query.starts_with('(') {
            find.toggle(FindOption::Regex, cx);
        }
    }

    fn fill(
        find: &gpui::Entity<FindWidget>,
        (query, with): (&'static str, &'static str),
        window: &mut gpui::VisualTestContext,
    ) {
        on_find(find, window, |find, window, cx| {
            find.open(Reveal::Replace, window, cx);
            regex_when_grouped(find, query, cx);
            find.find_input
                .update(cx, |input, cx| input.set_text(query, cx));
            find.replace_input
                .update(cx, |input, cx| input.set_text(with, cx));
        });
    }
}
