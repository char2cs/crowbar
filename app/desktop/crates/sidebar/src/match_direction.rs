//! Which way Enter steps through the search matches.

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(crate) enum MatchDirection {
    Forward,
    Backward,
}
