//! A keyboard move along the settings rail.

/// Where an arrow, Home or End key sends the rail selection.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RailMove {
    /// Arrow down or right.
    Next,
    /// Arrow up or left.
    Previous,
    /// Home.
    First,
    /// End.
    Last,
}
