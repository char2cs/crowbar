//! A daemon the shell reads from.

/// One host to follow: this computer's daemon, or a paired remote one.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct HostSpec {
    /// The transport's id for it: `local`, or the paired host's id.
    pub id: String,
    /// The name shown in the host picker.
    pub name: String,
    /// The `host:port` of a remote host; `None` for the local daemon.
    pub endpoint: Option<String>,
}
