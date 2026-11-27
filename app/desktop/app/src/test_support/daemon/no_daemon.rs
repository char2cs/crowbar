use protocol::{DaemonRequest, DaemonTransport, RelayEndpoint, RelayScope, Responder};

pub(crate) struct NoDaemon {
    answer: fn() -> protocol::Error,
    heard: Option<async_channel::Sender<String>>,
}

impl NoDaemon {
    pub(crate) fn timing_out() -> Self {
        Self {
            answer: || protocol::Error::Timeout,
            heard: None,
        }
    }

    pub(crate) fn telling(heard: async_channel::Sender<String>) -> Self {
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
            heard.try_send(line).unwrap_or_default();
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
