//! What the find bar asks of the terminal.

use terminal::SearchQuery;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum SearchBarEvent {
    QueryChanged(SearchQuery),
    Next,
    Previous,
    Close,
}
