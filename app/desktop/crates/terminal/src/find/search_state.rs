//! The matches of the active search.

use std::ops::RangeInclusive;

use alacritty_terminal::event::EventListener;
use alacritty_terminal::grid::Dimensions;
use alacritty_terminal::index::{Column, Direction, Line, Point};
use alacritty_terminal::term::Term;
use alacritty_terminal::term::search::{Match, RegexIter, RegexSearch};

use crate::error::Error;
use crate::find::search_query::SearchQuery;
use crate::find::search_summary::SearchSummary;
use crate::snapshot::Highlight;
use crate::snapshot::Viewport;

const MAX_MATCHES: usize = 10_000;

pub(crate) struct SearchState {
    query: SearchQuery,
    pattern: RegexSearch,
    matches: Vec<Match>,
    current: Option<usize>,
}

impl SearchState {
    pub(crate) fn start<T: EventListener>(
        term: &Term<T>,
        query: SearchQuery,
        from: Line,
    ) -> Result<Self, Error> {
        let pattern = RegexSearch::new(&query.pattern()).map_err(|error| Error::Pattern {
            reason: error.to_string(),
        })?;
        let mut state = Self {
            query,
            pattern,
            matches: Vec::new(),
            current: None,
        };
        state.refresh(term);
        state.current = state.first_from(from);
        Ok(state)
    }

    pub(crate) fn refresh<T: EventListener>(
        &mut self,
        term: &Term<T>,
    ) {
        self.matches = self.collect(term);
        self.current = self.current.filter(|index| *index < self.matches.len());
        if self.current.is_none() && !self.matches.is_empty() {
            self.current = Some(0);
        }
    }

    fn collect<T: EventListener>(
        &mut self,
        term: &Term<T>,
    ) -> Vec<Match> {
        if self.query.text.is_empty() {
            return Vec::new();
        }
        let start = Point::new(term.topmost_line(), Column(0));
        let end = Point::new(term.bottommost_line(), term.last_column());
        RegexIter::new(start, end, Direction::Right, term, &mut self.pattern)
            .take(MAX_MATCHES)
            .collect()
    }

    fn first_from(
        &self,
        from: Line,
    ) -> Option<usize> {
        let at_or_below = self
            .matches
            .iter()
            .position(|found| found.start().line >= from);
        at_or_below.or((!self.matches.is_empty()).then_some(0))
    }

    pub(crate) fn advance(&mut self) {
        self.step(1);
    }

    pub(crate) fn retreat(&mut self) {
        self.step(self.matches.len().saturating_sub(1));
    }

    fn step(
        &mut self,
        by: usize,
    ) {
        if self.matches.is_empty() {
            return;
        }
        self.current = Some((self.current.unwrap_or(0) + by) % self.matches.len());
    }

    pub(crate) fn current_match(&self) -> Option<&RangeInclusive<Point>> {
        self.matches.get(self.current?)
    }

    pub(crate) fn summary(&self) -> SearchSummary {
        SearchSummary {
            current: self.current.map_or(0, |index| index + 1),
            total: self.matches.len(),
        }
    }

    pub(crate) fn others(
        &self,
        viewport: &Viewport,
    ) -> Vec<Highlight> {
        self.matches
            .iter()
            .enumerate()
            .filter(|(index, _)| Some(*index) != self.current)
            .flat_map(|(_, found)| viewport.highlights(*found.start(), *found.end()))
            .collect()
    }

    pub(crate) fn current_highlights(
        &self,
        viewport: &Viewport,
    ) -> Vec<Highlight> {
        self.current_match().map_or_else(Vec::new, |found| {
            viewport.highlights(*found.start(), *found.end())
        })
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::find::search_options::SearchOptions;
    use crate::terminal::snapshot_builder::viewport_of;
    use crate::test_support::emulator_showing;

    fn started(text: &str) -> (Term<crate::terminal::EventSink>, SearchState) {
        let term = emulator_showing("cat dog cat", 20, 3);
        let query = SearchQuery {
            text: text.into(),
            options: SearchOptions::NONE,
        };
        let state =
            SearchState::start(&term, query, Line(0)).unwrap_or_else(|error| panic!("{error}"));
        (term, state)
    }

    #[test]
    fn matches_are_counted_stepped_and_highlighted() {
        let (term, mut state) = started("cat");
        assert_eq!((state.summary().current, state.summary().total), (1, 2));
        state.advance();
        assert_eq!(state.summary().current, 2);
        state.advance();
        assert_eq!(state.summary().current, 1);
        state.retreat();
        assert_eq!(state.summary().current, 2);
        let viewport = viewport_of(&term);
        assert_eq!(state.others(&viewport).len(), 1);
        assert_eq!(state.current_highlights(&viewport).len(), 1);
    }

    #[test]
    fn an_empty_query_matches_nothing_and_a_bad_pattern_fails() {
        let (_, state) = started("");
        assert_eq!(state.summary().total, 0);
        let term = emulator_showing("", 5, 2);
        let query = SearchQuery {
            text: "(".into(),
            options: SearchOptions::REGEX,
        };
        assert!(SearchState::start(&term, query, Line(0)).is_err());
    }
}
