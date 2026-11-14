use protocol::{DaemonResponse, Responder};

pub(crate) struct Reply(Option<Responder>);

impl Reply {
    pub(crate) fn new(respond: Responder) -> Self {
        Self(Some(respond))
    }

    pub(crate) fn send(mut self, response: DaemonResponse) {
        if let Some(respond) = self.0.take() {
            respond(Ok(response));
        }
    }
}

impl Drop for Reply {
    fn drop(&mut self) {
        if let Some(respond) = self.0.take() {
            respond(Err(protocol::Error::Interrupted));
        }
    }
}

#[cfg(test)]
#[path = "reply_tests.rs"]
mod tests;
