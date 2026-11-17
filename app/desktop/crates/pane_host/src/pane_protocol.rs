use std::sync::Arc;

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, Error as TransportError, HostId, HttpMethod,
    PaneOrigin, RelayEndpoint, RelayScope, RequestTarget,
};
use webview::{ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse};

use crate::status;

use crate::assignment::Assignment;
use crate::authorized::Authorized;
use crate::origin_table::OriginTable;
use crate::pane_bundles::PaneBundles;
use crate::policy::allows;
use crate::route::{Route, parse};

const CONTENT_TYPE: &str = "content-type";

/// Serves the `crowbar://` scheme: pane bundles from their [`BundleSource`](crate::BundleSource) and everything
/// else through the [`DaemonTransport`]. Every request is authorized against the origin of the
/// web view that made it.
pub struct PaneProtocol {
    scheme: String,
    transport: Arc<dyn DaemonTransport>,
    bundles: PaneBundles,
    origins: OriginTable,
}

impl std::fmt::Debug for PaneProtocol {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("PaneProtocol")
            .field("scheme", &self.scheme)
            .finish_non_exhaustive()
    }
}

impl PaneProtocol {
    /// A router for `scheme` (without `://`).
    #[must_use]
    pub fn new(
        scheme: impl Into<String>,
        transport: Arc<dyn DaemonTransport>,
        bundles: PaneBundles,
        origins: OriginTable,
    ) -> Self {
        Self {
            scheme: scheme.into(),
            transport,
            bundles,
            origins,
        }
    }

    fn serve(
        &self,
        call: Authorized,
        respond: ProtocolResponder,
    ) {
        let Authorized {
            request,
            pane,
            origin,
            route,
        } = call;
        match route {
            Route::CoreBundle { path } => respond(bundle_response(self.bundles.core.read(&path))),
            Route::ExtensionBundle { extension, path } => {
                let asset = self
                    .bundles
                    .extensions
                    .get(&extension)
                    .and_then(|source| source.read(&path));
                respond(bundle_response(asset));
            },
            Route::Relay { host } => self.relay(RelayScope::new(host, pane, origin), respond),
            Route::Daemon {
                host,
                path_and_query,
            } => {
                self.forward(
                    build_request(request, &origin, host, &path_and_query),
                    respond,
                );
            },
        }
    }
}

impl PaneProtocol {
    fn relay(
        &self,
        scope: RelayScope,
        respond: ProtocolResponder,
    ) {
        let transport = Arc::clone(&self.transport);
        let spawned = std::thread::Builder::new()
            .name("crowbar-relay".into())
            .spawn(move || respond(relay_response(transport.relay_endpoint(&scope))));
        if let Err(error) = spawned {
            tracing::error!(%error, "could not start the relay lookup");
        }
    }

    fn forward(
        &self,
        request: Option<DaemonRequest>,
        respond: ProtocolResponder,
    ) {
        match request {
            Some(forwarded) => self.transport.request(
                forwarded,
                Box::new(move |result| respond(daemon_response(result))),
            ),
            None => respond(error_response(status::BAD_REQUEST, "malformed request")),
        }
    }
}

impl ProtocolHandler for PaneProtocol {
    fn handle(
        &self,
        request: ProtocolRequest,
        respond: ProtocolResponder,
    ) {
        let Some(Assignment { pane, origin }) = self.origins.get(request.web_view) else {
            return respond(error_response(status::FORBIDDEN, "unknown web view"));
        };
        let Some(route) = parse(&self.scheme, &request.uri) else {
            return respond(error_response(status::BAD_REQUEST, "malformed crowbar url"));
        };
        if !allows(&origin, &route) {
            return respond(error_response(
                status::FORBIDDEN,
                "not allowed for this pane",
            ));
        }
        self.serve(
            Authorized {
                request,
                pane,
                origin,
                route,
            },
            respond,
        );
    }
}

fn build_request(
    request: ProtocolRequest,
    origin: &PaneOrigin,
    host: HostId,
    path_and_query: &str,
) -> Option<DaemonRequest> {
    let method = HttpMethod::new(request.method).ok()?;
    let target = RequestTarget::new(path_and_query).ok()?;
    DaemonRequest::new(host, origin.clone(), method, target)
        .with_headers(request.headers)
        .ok()
        .map(|forwarded| forwarded.with_body(request.body))
}

fn relay_response(endpoint: Result<RelayEndpoint, TransportError>) -> ProtocolResponse {
    let body = endpoint
        .map_err(|error| error.to_string())
        .and_then(|endpoint| serde_json::to_vec(&endpoint).map_err(|error| error.to_string()));
    match body {
        Ok(body) => ProtocolResponse {
            status: status::OK,
            headers: vec![(CONTENT_TYPE.into(), "application/json".into())],
            body,
        },
        Err(message) => error_response(status::BAD_GATEWAY, &message),
    }
}

fn bundle_response(asset: Option<crate::bundle_asset::BundleAsset>) -> ProtocolResponse {
    match asset {
        Some(asset) => ProtocolResponse {
            status: status::OK,
            headers: vec![
                (CONTENT_TYPE.into(), asset.mime.into()),
                ("cache-control".into(), "no-cache".into()),
            ],
            body: asset.bytes,
        },
        None => error_response(status::NOT_FOUND, "not found"),
    }
}

fn daemon_response(result: Result<DaemonResponse, TransportError>) -> ProtocolResponse {
    match result {
        Ok(response) => ProtocolResponse {
            status: response.status(),
            headers: response.headers().to_vec(),
            body: response.into_body(),
        },
        Err(TransportError::UnknownHost(_)) => error_response(status::NOT_FOUND, "unknown host"),
        Err(TransportError::Denied(reason)) => error_response(status::FORBIDDEN, reason),
        Err(TransportError::Timeout) => error_response(status::GATEWAY_TIMEOUT, "daemon timed out"),
        Err(other) => error_response(status::BAD_GATEWAY, &other.to_string()),
    }
}

fn error_response(
    status: u16,
    message: &str,
) -> ProtocolResponse {
    let body = serde_json::json!({ "error": message })
        .to_string()
        .into_bytes();
    ProtocolResponse {
        status,
        headers: vec![(CONTENT_TYPE.into(), "application/json".into())],
        body,
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;
    use std::sync::mpsc;

    use protocol::{DaemonResponse, ExtensionId, PaneId, PaneOrigin, RelayEndpoint, RelayScope};
    use webview::{ParentWindow, WebViewHost, WebViewId, WebViewKind};

    use super::*;
    use crate::bundle_asset::BundleAsset;
    use crate::bundle_source::BundleSource;
    use crate::extension_bundles::ExtensionBundles;
    use crate::test_support::blank_config;

    struct Recording {
        seen: Mutex<Vec<DaemonRequest>>,
        answer: Mutex<Option<protocol::Result<DaemonResponse>>>,
    }

    impl DaemonTransport for Recording {
        fn request(
            &self,
            request: DaemonRequest,
            respond: protocol::Responder,
        ) {
            self.seen.lock().unwrap().push(request);
            let answer = self.answer.lock().unwrap().take();
            respond(answer.unwrap_or(Err(TransportError::Timeout)));
        }

        fn relay_endpoint(
            &self,
            scope: &RelayScope,
        ) -> protocol::Result<RelayEndpoint> {
            Ok(RelayEndpoint::new(
                format!("ws://127.0.0.1:1/{}", scope.pane()),
                "token",
            ))
        }
    }

    struct Files;

    impl BundleSource for Files {
        fn read(
            &self,
            path: &str,
        ) -> Option<BundleAsset> {
            (path == "chat/index.html").then(|| BundleAsset {
                mime: "text/html",
                bytes: b"<p>".to_vec(),
            })
        }
    }

    fn blank_view(host: &WebViewHost) -> WebViewId {
        let config = blank_config(WebViewKind::Core);
        host.create_in(ParentWindow::X11(1), config).unwrap().id()
    }

    fn view_ids(count: usize) -> Vec<WebViewId> {
        let host = WebViewHost::with_platform(std::rc::Rc::new(webview::MockPlatform::default()));
        (0..count).map(|_| blank_view(&host)).collect()
    }

    struct Fixture {
        protocol: PaneProtocol,
        transport: Arc<Recording>,
        core: WebViewId,
        ext: WebViewId,
    }

    fn fixture() -> Fixture {
        let transport = Arc::new(Recording {
            seen: Mutex::new(Vec::new()),
            answer: Mutex::new(None),
        });
        let origins = OriginTable::default();
        let ids = view_ids(2);
        origins.set(ids[0], PaneId::new("p1").unwrap(), PaneOrigin::Core);
        origins.set(
            ids[1],
            PaneId::new("p2").unwrap(),
            PaneOrigin::Extension(ExtensionId::new("git").unwrap()),
        );
        let protocol = PaneProtocol::new(
            "crowbar",
            Arc::clone(&transport) as Arc<dyn DaemonTransport>,
            PaneBundles {
                core: Arc::new(Files),
                extensions: ExtensionBundles::default(),
            },
            origins,
        );
        Fixture {
            protocol,
            transport,
            core: ids[0],
            ext: ids[1],
        }
    }

    fn ask(
        f: &Fixture,
        view: WebViewId,
        uri: &str,
    ) -> ProtocolResponse {
        let (sender, receiver) = mpsc::channel();
        let request = ProtocolRequest {
            web_view: view,
            uri: uri.into(),
            method: "GET".into(),
            headers: Vec::new(),
            body: Vec::new(),
        };
        f.protocol.handle(
            request,
            Box::new(move |response| sender.send(response).unwrap()),
        );
        receiver.recv().unwrap()
    }

    #[test]
    fn core_bundle_files_are_served_with_their_media_type() {
        let f = fixture();
        let response = ask(&f, f.core, "crowbar://local/panes/core/chat/index.html");
        assert_eq!(response.status, 200, "status");
        assert!(
            response
                .headers
                .iter()
                .any(|(k, v)| k == CONTENT_TYPE && v == "text/html"),
            "mime"
        );
        assert_eq!(
            ask(&f, f.core, "crowbar://local/panes/core/nope.js").status,
            404,
            "missing file"
        );
    }

    #[test]
    fn daemon_requests_carry_the_origin_of_the_web_view() {
        let f = fixture();
        *f.transport.answer.lock().unwrap() =
            Some(Ok(DaemonResponse::new(200, Vec::new(), b"ok".to_vec())));
        let response = ask(&f, f.core, "crowbar://local/v0/health?x=1");
        assert_eq!(
            (response.status, response.body),
            (200, b"ok".to_vec()),
            "passed through"
        );
        let seen = f.transport.seen.lock().unwrap();
        assert_eq!(seen[0].target().as_str(), "/v0/health?x=1", "path");
        assert_eq!(seen[0].origin(), &PaneOrigin::Core, "origin");
    }

    #[test]
    fn extension_webviews_are_confined_to_their_own_routes() {
        let f = fixture();
        assert_eq!(
            ask(&f, f.ext, "crowbar://local/v0/threads").status,
            403,
            "general api"
        );
        assert_eq!(
            ask(&f, f.ext, "crowbar://local/panes/core/chat/index.html").status,
            403,
            "core bundle"
        );
        assert!(
            f.transport.seen.lock().unwrap().is_empty(),
            "nothing reached the daemon"
        );
        *f.transport.answer.lock().unwrap() =
            Some(Ok(DaemonResponse::new(204, Vec::new(), Vec::new())));
        assert_eq!(
            ask(&f, f.ext, "crowbar://local/v0/ext/git/review.diff").status,
            204,
            "own route"
        );
        let seen = f.transport.seen.lock().unwrap();
        assert_eq!(
            seen[0].origin().extension(),
            ExtensionId::new("git").ok().as_ref(),
            "extension id forwarded"
        );
    }

    #[test]
    fn unknown_views_and_bad_urls_are_refused() {
        let f = fixture();
        let stranger = view_ids(3)[2];
        assert_eq!(
            ask(&f, stranger, "crowbar://local/v0/x").status,
            403,
            "unknown view"
        );
        assert_eq!(
            ask(&f, f.core, "http://local/v0/x").status,
            400,
            "foreign scheme"
        );
    }

    #[test]
    fn transport_errors_map_to_http_statuses() {
        let f = fixture();
        let cases = [
            (TransportError::UnknownHost("x".to_owned()), 404),
            (TransportError::Denied("no"), 403),
            (TransportError::Timeout, 504),
            (TransportError::HostUnavailable("down".into()), 502),
        ];
        for (error, status) in cases {
            *f.transport.answer.lock().unwrap() = Some(Err(error));
            assert_eq!(ask(&f, f.core, "crowbar://local/v0/x").status, status);
        }
    }

    #[test]
    fn a_pane_gets_a_relay_endpoint_scoped_to_itself() {
        let f = fixture();
        let core = ask(&f, f.core, "crowbar://local/__crowbar/relay");
        let ext = ask(&f, f.ext, "crowbar://local/__crowbar/relay");
        let url_of = |response: &ProtocolResponse| {
            let value: serde_json::Value = serde_json::from_slice(&response.body).unwrap();
            value["url"].as_str().unwrap().to_owned()
        };
        assert_eq!(core.status, 200, "status");
        assert_eq!(url_of(&core), "ws://127.0.0.1:1/p1", "core pane");
        assert_eq!(url_of(&ext), "ws://127.0.0.1:1/p2", "extension pane");
    }
}
