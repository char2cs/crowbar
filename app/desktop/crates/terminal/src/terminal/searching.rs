use alacritty_terminal::index::Line;

use crate::error::Error;
use crate::find::SearchQuery;
use crate::find::SearchState;
use crate::find::SearchSummary;
use crate::terminal::Terminal;

impl Terminal {
    pub fn search(
        &mut self,
        query: SearchQuery,
    ) -> Result<SearchSummary, Error> {
        let top = Line(-i32::try_from(self.term.grid().display_offset()).unwrap_or(0));
        let state = SearchState::start(&self.term, query, top)?;
        let summary = state.summary();
        self.search = Some(state);
        Ok(summary)
    }

    pub fn search_next(&mut self) -> SearchSummary {
        self.step_search(SearchState::advance)
    }

    pub fn search_previous(&mut self) -> SearchSummary {
        self.step_search(SearchState::retreat)
    }

    fn step_search(
        &mut self,
        step: fn(&mut SearchState),
    ) -> SearchSummary {
        let Some(state) = self.search.as_mut() else {
            return SearchSummary::default();
        };
        step(state);
        if let Some(found) = state.current_match() {
            self.term.scroll_to_point(*found.start());
        }
        state.summary()
    }

    pub fn clear_search(&mut self) {
        self.search = None;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::find::SearchOptions;
    use crate::terminal::event::Event;
    use crate::terminal::wire::DaemonFrame;
    use crate::test_support::terminal_showing;

    fn shown(output: &str) -> Terminal {
        terminal_showing(output, 40, 6)
    }

    #[test]
    fn search_counts_and_steps_through_matches() {
        let mut terminal = shown("one two ONE");
        let query = SearchQuery {
            text: "one".into(),
            options: SearchOptions::NONE,
        };
        assert_eq!(
            terminal.search(query).map(|summary| summary.total).ok(),
            Some(2)
        );
        assert_eq!(terminal.search_next().current, 2);
        assert_eq!(terminal.search_previous().current, 1);
        terminal.clear_search();
        assert_eq!(
            terminal.snapshot().matches.len(),
            0,
            "clearing the search drops the highlights"
        );
    }

    #[test]
    fn a_batch_updates_the_search_matches_once_for_all_its_frames() {
        let mut terminal = shown("");
        let query = SearchQuery {
            text: "ab".into(),
            options: SearchOptions::NONE,
        };
        assert_eq!(terminal.search(query).map(|s| s.total).ok(), Some(0));
        let frames = [
            DaemonFrame::Output(b"ab ".to_vec()),
            DaemonFrame::Output(b"ab ".to_vec()),
            DaemonFrame::Exit(0),
        ];
        let events = terminal.apply_batch(&frames);
        assert_eq!(terminal.search_next().total, 2);
        assert_eq!(events, vec![Event::Exit(0)]);
    }

    #[test]
    fn queries_compile_or_are_refused() {
        let cases = [
            ("one", SearchOptions::NONE, true),
            ("(", SearchOptions::REGEX, false),
        ];
        for (text, options, accepted) in cases {
            assert_eq!(accepts(text, options), accepted, "{text}");
        }
    }

    fn accepts(
        text: &str,
        options: SearchOptions,
    ) -> bool {
        let query = SearchQuery {
            text: text.into(),
            options,
        };
        shown("one").search(query).is_ok()
    }
}
