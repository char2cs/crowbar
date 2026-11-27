use terminal::SearchQuery;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Event {
    QueryChanged(SearchQuery),
    Next,
    Previous,
    Close,
}
