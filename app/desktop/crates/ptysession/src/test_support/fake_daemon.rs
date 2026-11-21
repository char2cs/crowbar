//! A daemon that answers the terminal routes from memory and records every call.

use std::sync::{Arc, Mutex};

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, RelayEndpoint, RelayScope, Responder,
};
use tokio::sync::Notify;

/// The status of a created session.
pub(crate) const CREATED: u16 = 201;
/// The status of a refused request.
pub(crate) const FAILED: u16 = 500;
/// A list naming session `s1` as running.
pub(crate) const LISTED: &str = r#"{"success":true,"data":[{"id":"s1","status":"running"}]}"#;
/// A list naming no session.
pub(crate) const NONE_LISTED: &str = r#"{"success":true,"data":[]}"#;

const OK: u16 = 200;
const MADE: &str = r#"{"success":true,"data":{"sessionId":"s1"}}"#;
const EMPTY: &str = r#"{"success":true,"data":{}}"#;
/// What a daemon that refuses to kill a session says.
pub(crate) const KILL_REFUSED: &str = "injected";
const REFUSED: &str = r#"{"success":false,"error":"injected"}"#;

/// Creates session `s1` with `create_status`, lists `listed`, and notes each call.
pub(crate) struct FakeDaemon {
    pub(crate) seen: Mutex<Vec<String>>,
    pub(crate) deleted: Notify,
    create_status: u16,
    listed: &'static str,
    kill: (u16, &'static str),
}

/// A daemon that creates and lists session `s1`.
pub(crate) fn daemon() -> Arc<FakeDaemon> {
    daemon_answering(CREATED, LISTED)
}

/// A daemon that creates and lists session `s1` but refuses to kill it.
pub(crate) fn daemon_refusing_kills() -> Arc<FakeDaemon> {
    Arc::new(FakeDaemon {
        kill: (FAILED, REFUSED),
        ..fake(CREATED, LISTED)
    })
}

/// A daemon answering creation with `create_status` and the list with `listed`.
pub(crate) fn daemon_answering(
    create_status: u16,
    listed: &'static str,
) -> Arc<FakeDaemon> {
    Arc::new(fake(create_status, listed))
}

fn fake(
    create_status: u16,
    listed: &'static str,
) -> FakeDaemon {
    FakeDaemon {
        seen: Mutex::default(),
        deleted: Notify::new(),
        create_status,
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
            "POST" => (self.create_status, MADE),
            "DELETE" => self.kill,
            _ => (OK, EMPTY),
        };
        if method == "DELETE" {
            self.deleted.notify_one();
        }
        respond(Ok(DaemonResponse::new(status, Vec::new(), body.into())));
    }

    fn relay_endpoint(
        &self,
        _scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        Ok(RelayEndpoint::new("ws://relay".to_owned(), "tok"))
    }
}
