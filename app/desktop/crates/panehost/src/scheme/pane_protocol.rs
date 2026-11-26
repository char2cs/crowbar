use std::sync::Arc;

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, Error as TransportError, HostId, HttpMethod,
    PaneOrigin, RelayEndpoint, RelayScope, RequestTarget,
};
use webview::{ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse};

use crate::scheme::status;

use crate::catalog::PaneBundles;
use crate::scheme::access::authorized::Authorized;
use crate::scheme::access::page_caller::PageCaller;
use crate::scheme::access::policy::{self, allows};
use crate::scheme::menu::menu_route;
use crate::scheme::menu::pane_menus::PaneMenus;
use crate::scheme::origin_table::OriginTable;
use crate::scheme::origin_table::assignment::Assignment;
use crate::scheme::pane_tables::PaneTables;
use crate::scheme::route::{Route, parse};

const CONTENT_TYPE: &str = "content-type";

/// Serves the `crowbar://` scheme: pane bundles from their [`BundleSource`](crate::BundleSource) and everything
/// else through the [`DaemonTransport`]. Every request is authorized against the origin of the
/// web view that made it.
pub struct PaneProtocol {
    scheme: String,
    transport: Arc<dyn DaemonTransport>,
    bundles: PaneBundles,
    origins: OriginTable,
    menus: PaneMenus,
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
    /// A router for `scheme` (without `://`) over the `tables` it shares with its pane host.
    #[must_use]
    pub fn new(
        scheme: impl Into<String>,
        transport: Arc<dyn DaemonTransport>,
        bundles: PaneBundles,
        tables: PaneTables,
    ) -> Self {
        Self {
            scheme: scheme.into(),
            transport,
            bundles,
            origins: tables.origins,
            menus: tables.menus,
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
            Route::Menu => menu_route::ask(&self.menus, (pane, request), respond),
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
        let caller = policy::caller_of(&request);
        if caller == PageCaller::Stranger {
            return respond(error_response(status::FORBIDDEN, "origin not allowed"));
        }
        let respond: ProtocolResponder =
            Box::new(move |answer| respond(policy::readable_by(caller, answer)));
        let Some(Assignment { pane, origin }) = self.origins.get(request.web_view) else {
            return respond(error_response(status::FORBIDDEN, "unknown web view"));
        };
        let Some(route) = parse(&self.scheme, &request.uri) else {
            return respond(error_response(status::BAD_REQUEST, "malformed crowbar url"));
        };
        if !allows(&origin, &route) || !menu_route::may_ask(&request, &route) {
            return respond(error_response(
                status::FORBIDDEN,
                "not allowed for this pane",
            ));
        }
        if let Some(answer) = policy::preflight(&request) {
            return respond(answer);
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

fn bundle_response(asset: Option<crate::catalog::BundleAsset>) -> ProtocolResponse {
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
    use protocol::{DaemonResponse, ExtensionId};

    use super::*;
    use crate::test_support::{Served, header_of, request, served};

    fn answer_with(
        f: &Served,
        status: u16,
        body: &[u8],
    ) {
        let response = DaemonResponse::new(status, Vec::new(), body.to_vec());
        *f.transport.answer.lock().unwrap() = Some(Ok(response));
    }

    fn ask_from(
        f: &Served,
        asked: (&str, &str),
        headers: &[(&str, &str)],
    ) -> ProtocolResponse {
        f.ask(request(f.core, asked, headers)).recv().unwrap()
    }

    #[test]
    fn a_document_of_any_other_origin_is_refused_before_the_daemon() {
        let f = served();
        let strangers = ["null", "crowbar://host-1", "https://evil.example"];
        let asks = strangers
            .into_iter()
            .flat_map(|from| [("POST", from), ("OPTIONS", from)]);
        for (method, stranger) in asks {
            let asked = [
                ("Origin", stranger),
                ("Access-Control-Request-Method", "POST"),
            ];
            let answer = ask_from(&f, (method, "crowbar://local/v0/projects"), &asked);
            let granted = header_of(&answer, "access-control-allow-origin");
            assert_eq!(
                (answer.status, granted),
                (403, None),
                "{method} from {stranger}"
            );
        }
        assert!(
            f.transport.seen.lock().unwrap().is_empty(),
            "nothing reached the daemon"
        );
    }

    #[test]
    fn the_shells_page_is_granted_its_own_origin_and_the_headers_panes_send() {
        let f = served();
        answer_with(&f, 200, b"ok");
        let origin = ("Origin", "crowbar://local");
        let answer = ask_from(&f, ("GET", "crowbar://host-1/v0/health"), &[origin]);
        let grants = [
            "access-control-allow-origin",
            "access-control-expose-headers",
            "vary",
        ]
        .map(|grant| header_of(&answer, grant));
        assert_eq!(answer.status, 200);
        assert_eq!(grants, [Some("crowbar://local"), Some("*"), Some("Origin")]);
        let asked = [
            origin,
            ("Access-Control-Request-Method", "POST"),
            ("Access-Control-Request-Headers", "content-type, x-evil"),
        ];
        let preflight = ask_from(&f, ("OPTIONS", "crowbar://host-1/v0/projects"), &asked);
        assert_eq!(preflight.status, 204);
        assert_eq!(
            header_of(&preflight, "access-control-allow-headers"),
            Some("content-type, idempotency-key, crowbar-discard-work"),
            "a fixed list, never the request's echo"
        );
    }

    #[test]
    fn core_bundle_files_are_served_with_their_media_type() {
        let f = served();
        let page = f.get(f.core, "crowbar://local/panes/core/chat/index.html");
        let mime = header_of(&page, CONTENT_TYPE);
        assert_eq!((page.status, mime), (200, Some("text/html")), "a file");
        let missing = f.get(f.core, "crowbar://local/panes/core/nope.js");
        assert_eq!(missing.status, 404, "missing file");
    }

    #[test]
    fn daemon_requests_carry_the_origin_of_the_web_view() {
        let f = served();
        answer_with(&f, 200, b"ok");
        let response = f.get(f.core, "crowbar://local/v0/health?x=1");
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
        let f = served();
        let general = f.get(f.ext, "crowbar://local/v0/threads").status;
        let core = f
            .get(f.ext, "crowbar://local/panes/core/chat/index.html")
            .status;
        assert_eq!((general, core), (403, 403), "general api, core bundle");
        assert!(
            f.transport.seen.lock().unwrap().is_empty(),
            "nothing reached the daemon"
        );
        answer_with(&f, 204, b"");
        let own = f.get(f.ext, "crowbar://local/v0/ext/git/review.diff");
        assert_eq!(own.status, 204, "own route");
        let seen = f.transport.seen.lock().unwrap();
        let git = ExtensionId::new("git").ok();
        assert_eq!(
            seen[0].origin().extension(),
            git.as_ref(),
            "extension id forwarded"
        );
    }

    #[test]
    fn unknown_views_bad_urls_and_transport_errors_map_to_http_statuses() {
        let f = served();
        assert_eq!(
            f.get(f.stranger, "crowbar://local/v0/x").status,
            403,
            "unknown view"
        );
        assert_eq!(
            f.get(f.core, "http://local/v0/x").status,
            400,
            "foreign scheme"
        );
        let cases = [
            (TransportError::UnknownHost("x".to_owned()), 404),
            (TransportError::Denied("no"), 403),
            (TransportError::Timeout, 504),
            (TransportError::HostUnavailable("down".into()), 502),
        ];
        for (error, status) in cases {
            *f.transport.answer.lock().unwrap() = Some(Err(error));
            assert_eq!(f.get(f.core, "crowbar://local/v0/x").status, status);
        }
    }

    #[test]
    fn a_pane_gets_a_relay_endpoint_scoped_to_itself() {
        let f = served();
        let url_of = |view| {
            let response = f.get(view, "crowbar://local/__crowbar/relay");
            let value: serde_json::Value = serde_json::from_slice(&response.body).unwrap();
            (response.status, value["url"].as_str().unwrap().to_owned())
        };
        assert_eq!(
            url_of(f.core),
            (200, "ws://127.0.0.1:1/p1".into()),
            "core pane"
        );
        assert_eq!(
            url_of(f.ext),
            (200, "ws://127.0.0.1:1/p2".into()),
            "extension pane"
        );
    }
}
