use std::sync::Arc;

use protocol::{DaemonRequest, DaemonResponse, DaemonTransport, Error as TransportError, HostId};
use webview::{ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse};

use crate::bundle_source::BundleSource;
use crate::extension_bundles::ExtensionBundles;
use crate::origin::Origin;
use crate::origin_table::OriginTable;
use crate::policy::allows;
use crate::route::{Route, parse};

/// Serves the `crowbar://` scheme: pane bundles from their [`BundleSource`] and everything
/// else through the [`DaemonTransport`]. Every request is authorized against the origin of the
/// web view that made it.
pub struct PaneProtocol {
    scheme: String,
    transport: Arc<dyn DaemonTransport>,
    core_bundle: Arc<dyn BundleSource>,
    extension_bundles: ExtensionBundles,
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
        core_bundle: Arc<dyn BundleSource>,
        extension_bundles: ExtensionBundles,
        origins: OriginTable,
    ) -> Self {
        Self {
            scheme: scheme.into(),
            transport,
            core_bundle,
            extension_bundles,
            origins,
        }
    }

    fn serve(
        &self,
        request: ProtocolRequest,
        origin: &Origin,
        route: Route,
        respond: ProtocolResponder,
    ) {
        match route {
            Route::CoreBundle { path } => respond(bundle_response(self.core_bundle.read(&path))),
            Route::ExtensionBundle { extension, path } => {
                let asset = self
                    .extension_bundles
                    .get(&extension)
                    .and_then(|source| source.read(&path));
                respond(bundle_response(asset));
            },
            Route::Daemon {
                host,
                path_and_query,
            } => {
                self.forward(request, origin, host, path_and_query, respond);
            },
        }
    }

    fn forward(
        &self,
        request: ProtocolRequest,
        origin: &Origin,
        host: HostId,
        path_and_query: String,
        respond: ProtocolResponder,
    ) {
        let forwarded = DaemonRequest {
            host,
            origin: origin.kind,
            extension: origin.extension.clone(),
            method: request.method,
            path_and_query,
            headers: request.headers,
            body: request.body,
        };
        self.transport.request(
            forwarded,
            Box::new(move |result| respond(daemon_response(result))),
        );
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
        self.serve(request, &origin, route, respond);
    }
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
            status: response.status,
            headers: response.headers,
            body: response.body,
        },
        Err(TransportError::UnknownHost(_)) => error_response(404, "unknown host"),
        Err(TransportError::Denied(reason)) => error_response(403, &reason),
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
