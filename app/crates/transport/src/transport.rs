use std::sync::Arc;
use std::time::Duration;

use http::{HeaderName, HeaderValue, Request, Response, header};
use protocol::HostId;
use tokio::sync::broadcast;

use crate::access_scope::{AccessScope, EXTENSION_HEADER};
use crate::error::Error;
use crate::host_registry::HostRegistry;
use crate::host_target::HostTarget;
use crate::local::{connect_retry_delay, forward_local};
use crate::remote::forward_remote;
use crate::response::{error_response, readable_by};
use crate::socket_locator::SocketLocator;
use crate::token_store::TokenStore;
use crate::upstream::{self, UpstreamSocket};

/// Upper bound on one proxied request: a daemon that accepts but never answers would otherwise
/// pin a socket and two tasks for the app's life (`fetch` never aborts). Well above the daemon's
/// own slowest bound, a 3-minute network git transfer.
pub const PROXY_TIMEOUT: Duration = Duration::from_secs(300);

/// Reaches the local daemon and paired remote hosts on behalf of panes.
pub struct Transport {
    locator: Arc<dyn SocketLocator>,
    registry: Arc<HostRegistry>,
    tokens: Arc<dyn TokenStore>,
    removed: broadcast::Sender<HostId>,
}

impl std::fmt::Debug for Transport {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("Transport")
            .field("registry", &self.registry)
            .finish_non_exhaustive()
    }
}

impl Transport {
    /// Wires the local socket source, the host list and the credential store together.
    pub fn new(
        locator: Arc<dyn SocketLocator>,
        registry: Arc<HostRegistry>,
        tokens: Arc<dyn TokenStore>,
    ) -> Self {
        let (removed, _) = broadcast::channel(16);
        Self {
            locator,
            registry,
            tokens,
            removed,
        }
    }

    /// The host list.
    #[must_use]
    pub fn registry(&self) -> &Arc<HostRegistry> {
        &self.registry
    }

    /// The credential store.
    #[must_use]
    pub fn tokens(&self) -> &Arc<dyn TokenStore> {
        &self.tokens
    }

    pub(crate) fn locator(&self) -> &dyn SocketLocator {
        self.locator.as_ref()
    }

    /// Drops a host so nothing can reach it, and tells every open stream to it to end.
    ///
    /// # Errors
    /// A hosts-file error when the list cannot be saved; the host is then still registered.
    pub fn remove_host(&self, host_id: &HostId) -> Result<(), Error> {
        self.registry.remove(host_id)?;
        self.removed.send(host_id.clone()).ok();
        Ok(())
    }

    pub(crate) fn removed_hosts(&self) -> broadcast::Receiver<HostId> {
        self.removed.subscribe()
    }

    /// Answers one `crowbar://<hostId>/v0/...` request made by a pane with reach `scope`. The
    /// answer is always a response: a failure is a marked proxy answer the page can read.
    pub async fn handle(
        &self,
        scope: &AccessScope,
        request: Request<Vec<u8>>,
    ) -> Response<Vec<u8>> {
        let origin = request.headers().get(header::ORIGIN).cloned();
        let response = match tokio::time::timeout(PROXY_TIMEOUT, self.route(scope, request)).await {
            Ok(response) => response,
            Err(_) => error_response(504, "crowbar daemon did not answer in time"),
        };
        readable_by(origin, response)
    }

    async fn route(&self, scope: &AccessScope, mut request: Request<Vec<u8>>) -> Response<Vec<u8>> {
        let target_text = request.uri().path_and_query().map_or_else(
            || request.uri().path().to_owned(),
            |pq| pq.as_str().to_owned(),
        );
        if scope.check(&target_text).is_err() {
            return error_response(403, "crowbar api is not available to this origin");
        }
        let target = match target_of(request.uri()) {
            Ok(target) => target,
            Err(e) => return error_response(400, &e.to_string()),
        };
        stamp_scope(request.headers_mut(), scope);
        match target {
            HostTarget::Local => self.forward_to_local(request).await,
            HostTarget::Remote(id) => {
                forward_remote(&self.registry, &self.tokens, &id, request).await
            },
        }
    }

    async fn forward_to_local(&self, request: Request<Vec<u8>>) -> Response<Vec<u8>> {
        forward_local(self.locator.as_ref(), request, |attempt| {
            tokio::time::sleep(connect_retry_delay(attempt))
        })
        .await
    }

    pub(crate) async fn open_stream(
        &self,
        target: &HostTarget,
        path_and_query: &str,
        max_message: usize,
    ) -> Result<UpstreamSocket, Error> {
        upstream::open(self, target, path_and_query, max_message).await
    }
}

fn target_of(uri: &http::Uri) -> Result<HostTarget, Error> {
    match uri.host() {
        None | Some("" | "localhost") => Ok(HostTarget::Local),
        Some(authority) => Ok(HostTarget::of(&HostId::new(authority)?)),
    }
}

fn stamp_scope(headers: &mut http::HeaderMap, scope: &AccessScope) {
    let name = HeaderName::from_static(EXTENSION_HEADER);
    headers.remove(&name);
    let id = scope
        .extension()
        .and_then(|id| HeaderValue::from_str(id.as_str()).ok());
    if let Some(value) = id {
        headers.insert(name, value);
    }
}

#[cfg(test)]
#[path = "transport_tests.rs"]
mod tests;
