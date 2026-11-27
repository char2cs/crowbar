use std::sync::{Arc, Mutex};

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, RelayEndpoint, RelayScope, Responder,
};
use serde_json::Value;

#[derive(Debug)]
struct Route {
    method: &'static str,
    path: String,
    status: u16,
    headers: Vec<(String, String)>,
    body: String,
}

#[derive(Debug, Clone, PartialEq)]
pub struct Seen {
    pub method: String,
    pub path: String,
    pub body: Option<Value>,
    pub headers: Vec<(String, String)>,
}

#[derive(Debug, Default)]
pub struct FakeDaemon {
    routes: Mutex<Vec<Route>>,
    seen: Mutex<Vec<Seen>>,
}

impl FakeDaemon {
    pub fn shared() -> Arc<Self> {
        Arc::new(Self::default())
    }

    pub fn answer(
        &self,
        (method, path): (&'static str, &str),
        status: u16,
        body: &str,
    ) {
        self.route(Route {
            method,
            path: path.to_owned(),
            status,
            headers: Vec::new(),
            body: body.to_owned(),
        });
    }

    pub fn proxied(
        &self,
        (method, path): (&'static str, &str),
        status: u16,
        marker: &str,
    ) {
        self.route(Route {
            method,
            path: path.to_owned(),
            status,
            headers: vec![(protocol::PROXY_HEADER.to_owned(), marker.to_owned())],
            body: format!("the transport answered for the host: {marker}"),
        });
    }

    fn route(
        &self,
        route: Route,
    ) {
        if let Ok(mut routes) = self.routes.lock() {
            routes.retain(|held| !(held.method == route.method && held.path == route.path));
            routes.push(route);
        }
    }

    pub fn ok(
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

    pub fn seen(&self) -> Vec<Seen> {
        self.seen.lock().map_or_default(|seen| seen.clone())
    }

    fn find(
        &self,
        method: &str,
        path: &str,
    ) -> DaemonResponse {
        let routes = self.routes.lock();
        let hit = routes
            .iter()
            .flat_map(|routes| routes.iter())
            .find(|route| route.method == method && route.path == path);
        hit.map_or_else(
            || {
                let missing = r#"{"success":false,"error":"no such route"}"#;
                DaemonResponse::new(404, Vec::new(), missing.as_bytes().to_vec())
            },
            |route| {
                let body = route.body.clone().into_bytes();
                DaemonResponse::new(route.status, route.headers.clone(), body)
            },
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
        respond(Ok(self.find(method, path)));
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
