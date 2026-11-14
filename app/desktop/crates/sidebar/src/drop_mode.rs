//! Where a dragged row lands relative to the row under the pointer.

/// Before, after or into the target row.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub enum DropMode {
    /// A slot above the target.
    Before,
    /// A slot below the target.
    After,
    /// Nested inside the target.
    Into,
}
