use std::sync::{Arc, Mutex};

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, HostId, RelayEndpoint, RelayScope, Responder,
};
use serde_json::Value;

struct Route {
    method: &'static str,
    path: String,
    status: u16,
    body: String,
}

/// What the fake daemon saw.
#[derive(Debug, Clone, PartialEq)]
pub(crate) struct Seen {
    pub(crate) method: String,
    pub(crate) path: String,
    pub(crate) body: Option<Value>,
    pub(crate) headers: Vec<(String, String)>,
}

/// A [`DaemonTransport`] answering from a table of routes, recording every request.
#[derive(Default)]
pub(crate) struct FakeDaemon {
    routes: Mutex<Vec<Route>>,
    seen: Mutex<Vec<Seen>>,
}

impl FakeDaemon {
    pub(crate) fn shared() -> Arc<Self> {
        Arc::new(Self::default())
    }

    pub(crate) fn answer(
        &self,
        (method, path): (&'static str, &str),
        status: u16,
        body: &str,
    ) {
        let route = Route {
            method,
            path: path.to_owned(),
            status,
            body: body.to_owned(),
        };
        if let Ok(mut routes) = self.routes.lock() {
            routes.retain(|held| !(held.method == method && held.path == path));
            routes.push(route);
        }
    }

    pub(crate) fn ok(
        &self,
        method: &'static str,
        path: &str,
        data: &str,
    ) {
        self.answer(
            (method, path),
            200,
            &format!(r#"{{"success":true,"data":{data}}}"#),
        );
    }

    pub(crate) fn seen(&self) -> Vec<Seen> {
        self.seen.lock().map_or_default(|seen| seen.clone())
    }

    fn find(
        &self,
        method: &str,
        path: &str,
    ) -> (u16, String) {
        let routes = self.routes.lock();
        let hit = routes
            .iter()
            .flat_map(|routes| routes.iter())
            .find(|route| route.method == method && route.path == path);
        hit.map_or_else(
            || {
                (
                    404,
                    r#"{"success":false,"error":"no such route"}"#.to_owned(),
                )
            },
            |route| (route.status, route.body.clone()),
        )
    }
}

impl DaemonTransport for FakeDaemon {
    fn request(
        &self,
        request: DaemonRequest,
        respond: Responder,
    ) {
        let (method, path) = (request.method().as_str(), request.target().as_str());
        let entry = Seen {
            method: method.to_owned(),
            path: path.to_owned(),
            body: serde_json::from_slice(request.body()).ok(),
            headers: request.headers().to_vec(),
        };
        if let Ok(mut seen) = self.seen.lock() {
            seen.push(entry);
        }
        let (status, body) = self.find(method, path);
        respond(Ok(DaemonResponse::new(
            status,
            Vec::new(),
            body.into_bytes(),
        )));
    }

    fn relay_endpoint(
        &self,
        scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        Err(protocol::Error::UnknownHost(
            scope.host().as_str().to_owned(),
        ))
    }
}

pub(crate) fn local() -> HostId {
    HostId::new("local").unwrap_or_else(|error| panic!("local is a valid host id: {error}"))
}
