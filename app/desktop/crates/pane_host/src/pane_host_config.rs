use protocol::HostId;

/// Static settings of a [`PaneHost`](crate::PaneHost).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PaneHostConfig {
    /// The custom scheme, without `://`.
    pub scheme: String,
    /// The daemon host panes talk to.
    pub host: HostId,
    /// Free core web views kept for reuse; more are destroyed.
    pub max_idle_slots: usize,
}
