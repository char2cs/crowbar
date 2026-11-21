use std::sync::mpsc;

use protocol::{DaemonRequest, DaemonTransport, RelayEndpoint, RelayScope, Responder};

pub(crate) struct NoDaemon {
    answer: fn() -> protocol::Error,
    heard: Option<mpsc::Sender<String>>,
}

impl NoDaemon {
    pub(crate) fn timing_out() -> Self {
        Self {
            answer: || protocol::Error::Timeout,
            heard: None,
        }
    }

    /// Times out like [`NoDaemon::timing_out`], telling `heard` each request as "METHOD target".
    pub(crate) fn telling(heard: mpsc::Sender<String>) -> Self {
        Self {
            heard: Some(heard),
            ..Self::timing_out()
        }
    }
}

impl DaemonTransport for NoDaemon {
    fn request(
        &self,
        request: DaemonRequest,
        respond: Responder,
    ) {
        if let Some(heard) = &self.heard {
            let line = format!(
                "{} {}",
                request.method().as_str(),
                request.target().as_str()
            );
            heard.send(line).unwrap_or_default();
        }
        respond(Err((self.answer)()));
    }

    fn relay_endpoint(
        &self,
        _scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        Err((self.answer)())
    }
}
