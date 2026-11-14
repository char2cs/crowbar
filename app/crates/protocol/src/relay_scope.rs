use crate::host_id::HostId;
use crate::pane_id::PaneId;
use crate::pane_origin::PaneOrigin;

/// Who a WebSocket relay endpoint is for: one pane, of one origin, talking to one host.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct RelayScope {
    host: HostId,
    pane: PaneId,
    origin: PaneOrigin,
}

impl RelayScope {
    /// The scope of `pane`, of `origin`, on `host`.
    #[must_use]
    pub fn new(host: HostId, pane: PaneId, origin: PaneOrigin) -> Self {
        Self { host, pane, origin }
    }

    /// The daemon host the stream connects to.
    #[must_use]
    pub fn host(&self) -> &HostId {
        &self.host
    }

    /// The pane the token is issued to.
    #[must_use]
    pub fn pane(&self) -> &PaneId {
        &self.pane
    }

    /// Whether the pane is core or which extension it belongs to.
    #[must_use]
    pub fn origin(&self) -> &PaneOrigin {
        &self.origin
    }
}

#[cfg(test)]
#[path = "relay_scope_tests.rs"]
mod tests;
