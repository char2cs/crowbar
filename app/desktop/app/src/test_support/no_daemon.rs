use protocol::{DaemonRequest, DaemonTransport, RelayEndpoint, RelayScope, Responder};

pub(crate) struct NoDaemon {
    answer: fn() -> protocol::Error,
}

impl NoDaemon {
    pub(crate) fn timing_out() -> Self {
        Self {
            answer: || protocol::Error::Timeout,
        }
    }
}

impl DaemonTransport for NoDaemon {
    fn request(
        &self,
        _request: DaemonRequest,
        respond: Responder,
    ) {
        respond(Err((self.answer)()));
    }

    fn relay_endpoint(
        &self,
        _scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        Err((self.answer)())
    }
}
