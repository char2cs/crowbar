//! How much a question's accept button risks.

/// What accepting a [`crate::Question`] costs, which picks its accept button's colour.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Stakes {
    /// Work is kept, as a detach keeps it: the primary button.
    Disruptive,
    /// Something running is lost, as a kill loses the turn: the destructive button.
    Destructive,
}
