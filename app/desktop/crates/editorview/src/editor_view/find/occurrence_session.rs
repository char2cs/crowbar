use std::ops::Range;

use editor::FindQuery;

#[derive(Debug, Clone)]
pub(crate) struct OccurrenceSession {
    pub(crate) query: FindQuery,
    pub(crate) selections: Vec<Range<usize>>,
}
