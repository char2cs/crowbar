//! Whether the first snapshot of the projects scope has arrived.

/// Before the snapshot the sidebar shows skeleton bars.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ScopeSync {
    /// Nothing received yet.
    Pending,
    /// The scope is synced.
    Ready,
}
