//! The outcome of a search, for the find bar.

/// How many matches there are and which one is current.
#[derive(Debug, Clone, Copy, Default, PartialEq, Eq)]
pub struct SearchSummary {
    /// One-based index of the current match; 0 when there is none.
    pub current: usize,
    /// Matches in the scrollback and screen.
    pub total: usize,
}
