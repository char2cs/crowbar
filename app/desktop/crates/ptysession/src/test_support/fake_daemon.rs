use std::sync::{Arc, Mutex};

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, RelayEndpoint, RelayScope, Responder,
};

pub(crate) const CREATED: u16 = 201;
pub(crate) const FAILED: u16 = 500;
pub(crate) const LISTED: &str = r#"{"success":true,"data":[{"id":"s1","status":"running"}]}"#;
pub(crate) const NONE_LISTED: &str = r#"{"success":true,"data":[]}"#;

const OK: u16 = 200;
const MADE: &str = r#"{"success":true,"data":{"sessionId":"s1"}}"#;
const EMPTY: &str = r#"{"success":true,"data":{}}"#;
pub(crate) const KILL_REFUSED: &str = "injected";
const REFUSED: &str = r#"{"success":false,"error":"injected"}"#;

pub(crate) struct FakeDaemon {
    pub(crate) seen: Mutex<Vec<String>>,
    create: (u16, &'static str),
    listed: &'static str,
    kill: (u16, &'static str),
}

pub(crate) fn daemon() -> Arc<FakeDaemon> {
    daemon_answering(CREATED, LISTED)
}

pub(crate) fn daemon_refusing_kills() -> Arc<FakeDaemon> {
    Arc::new(FakeDaemon {
        kill: (FAILED, REFUSED),
        ..fake(CREATED, LISTED)
    })
}

pub(crate) fn daemon_answering(
    create_status: u16,
    listed: &'static str,
) -> Arc<FakeDaemon> {
    Arc::new(fake(create_status, listed))
}

pub(crate) fn daemon_creating(
    status: u16,
    body: &'static str,
) -> Arc<FakeDaemon> {
    Arc::new(FakeDaemon {
        create: (status, body),
        ..fake(CREATED, LISTED)
    })
}

fn fake(
    create_status: u16,
    listed: &'static str,
) -> FakeDaemon {
    FakeDaemon {
        seen: Mutex::default(),
        create: (create_status, MADE),
        listed,
        kill: (OK, EMPTY),
    }
}

impl DaemonTransport for FakeDaemon {
    fn request(
        &self,
        request: DaemonRequest,
        respond: Responder,
    ) {
        let method = request.method().as_str().to_owned();
        let line = format!("{method} {}", request.target().as_str());
        self.seen.lock().unwrap().push(line);
        let (status, body) = match method.as_str() {
            "GET" => (OK, self.listed),
            "POST" => self.create,
            "DELETE" => self.kill,
            _ => (OK, EMPTY),
        };
        respond(Ok(DaemonResponse::new(status, Vec::new(), body.into())));
    }

    fn relay_endpoint(
        &self,
        _scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        Ok(RelayEndpoint::new("ws://relay".to_owned(), "tok"))
    }
}
