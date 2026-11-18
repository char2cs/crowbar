//! Whether a row is shown in a pane.

/// Open rows rest on the idle fill.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Presence {
    /// No view is open for the row.
    #[default]
    Absent,
    /// A pane or view shows the row.
    Open,
}
