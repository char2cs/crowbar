//! The colour family of a host status.

/// Online, offline, needing attention or not yet settled.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HostTone {
    /// Green dot.
    Online,
    /// Red dot.
    Offline,
    /// Amber dot.
    NeedsAttention,
    /// Hollow ring.
    Unsettled,
}
