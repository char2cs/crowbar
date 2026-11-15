use std::sync::Arc;

use protocol::{DaemonRequest, DaemonResponse, DaemonTransport, Error as TransportError, HostId, HttpMethod, PaneOrigin, RequestTarget};
use webview::{ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse};

use crate::authorized::Authorized;
use crate::pane_bundles::PaneBundles;
use crate::origin_table::OriginTable;
use crate::policy::allows;
use crate::route::{Route, parse};

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
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
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

    fn serve(&self, call: Authorized, respond: ProtocolResponder) {
        let Authorized {
            request,
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
            Route::Daemon {
                host,
                path_and_query,
            } => {
                self.forward(build_request(request, &origin, host, &path_and_query), respond);
            },
        }
    }
}

impl PaneProtocol {
    fn forward(&self, request: Option<DaemonRequest>, respond: ProtocolResponder) {
        match request {
            Some(forwarded) => self.transport.request(
                forwarded,
                Box::new(move |result| respond(daemon_response(result))),
            ),
            None => respond(error_response(400, "malformed request")),
        }
    }
}

impl ProtocolHandler for PaneProtocol {
    fn handle(&self, request: ProtocolRequest, respond: ProtocolResponder) {
        let Some(origin) = self.origins.get(request.web_view) else {
            return respond(error_response(403, "unknown web view"));
        };
        let Some(route) = parse(&self.scheme, &request.uri) else {
            return respond(error_response(400, "malformed crowbar url"));
        };
        if !allows(&origin, &route) {
            return respond(error_response(403, "not allowed for this pane"));
        }
        self.serve(
            Authorized {
                request,
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

fn bundle_response(asset: Option<crate::bundle_asset::BundleAsset>) -> ProtocolResponse {
    match asset {
        Some(asset) => ProtocolResponse {
            status: 200,
            headers: vec![
                ("content-type".into(), asset.mime.into()),
                ("cache-control".into(), "no-cache".into()),
            ],
            body: asset.bytes,
        },
        None => error_response(404, "not found"),
    }
}

fn daemon_response(result: Result<DaemonResponse, TransportError>) -> ProtocolResponse {
    match result {
        Ok(response) => ProtocolResponse {
            status: response.status(),
            headers: response.headers().to_vec(),
            body: response.into_body(),
        },
        Err(TransportError::UnknownHost(_)) => error_response(404, "unknown host"),
        Err(TransportError::Denied(reason)) => error_response(403, reason),
        Err(TransportError::Timeout) => error_response(504, "daemon timed out"),
        Err(other) => error_response(502, &other.to_string()),
    }
}

fn error_response(status: u16, message: &str) -> ProtocolResponse {
    let body = serde_json::json!({ "error": message })
        .to_string()
        .into_bytes();
    ProtocolResponse {
        status,
        headers: vec![("content-type".into(), "application/json".into())],
        body,
    }
}

#[cfg(test)]
#[path = "pane_protocol_tests.rs"]
mod tests;
