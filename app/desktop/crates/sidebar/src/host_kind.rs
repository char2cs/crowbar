//! Whether a host is this machine.

/// Local or remote.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum HostKind {
    /// The daemon on this machine.
    Local,
    /// A paired remote daemon.
    Remote,
}
