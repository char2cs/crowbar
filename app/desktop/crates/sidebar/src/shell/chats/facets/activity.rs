//! Whether an agent turn is running for a row.

/// A working row shows a spinner in place of its glyph.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum Activity {
    /// Nothing running.
    #[default]
    Idle,
    /// An agent turn or an operation is running.
    Working,
}
