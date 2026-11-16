//! What the merge box is busy with.

/// The request the merge box is waiting on; the box offers one at a time.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
#[non_exhaustive]
pub enum MergeWork {
    /// Nothing is running.
    #[default]
    Idle,
    /// The commit of the listed files is running: its button reads "Committing…" and is disabled.
    Committing,
    /// A merge is running: its button reads "Merging…" and is disabled.
    Merging,
}
