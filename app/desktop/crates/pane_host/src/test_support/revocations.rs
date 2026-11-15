use std::sync::{Mutex, PoisonError};

use protocol::{DaemonRequest, DaemonTransport, PaneId, RelayEndpoint, RelayScope, Responder};

#[derive(Debug, Default)]
pub(crate) struct Revocations {
    panes: Mutex<Vec<PaneId>>,
}

impl Revocations {
    pub(crate) fn panes(&self) -> Vec<PaneId> {
        self.panes
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .clone()
    }
}

impl DaemonTransport for Revocations {
    fn request(
        &self,
        _request: DaemonRequest,
        respond: Responder,
    ) {
        respond(Err(protocol::Error::Timeout));
    }

    fn relay_endpoint(
        &self,
        _scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        Err(protocol::Error::Timeout)
    }

    fn revoke_pane(
        &self,
        scope: &RelayScope,
    ) {
        self.panes
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .push(scope.pane().clone());
    }
}
