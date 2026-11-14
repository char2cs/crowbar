use std::sync::Arc;
use std::time::Duration;

use http::{Request, Response, header};
use protocol::{HostId, PaneOrigin};
use tokio::sync::broadcast;

use crate::access_scope;
use crate::credential_io;
use crate::error::Error;
use crate::host_registry::HostRegistry;
use crate::host_target::HostTarget;
use crate::http_conn::strip_pane_headers;
use crate::local::{connect_retry_delay, forward_local};
use crate::remote::forward_remote;
use crate::response::{error_response, readable_by};
use crate::socket_locator::SocketLocator;
use crate::token_store::TokenStore;
use crate::upstream::{self, UpstreamSocket};
use crate::upstream_request::UpstreamRequest;

pub(crate) const PROXY_TIMEOUT: Duration = Duration::from_secs(300);
const REMOVAL_BACKLOG: usize = 16;

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
        let (removed, _) = broadcast::channel(REMOVAL_BACKLOG);
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

    /// Forgets a host: it is dropped from the list so nothing can reach it, every open stream to
    /// it is told to end, and its stored credential is deleted. Safe to call again after a
    /// failure.
    ///
    /// # Errors
    /// A hosts-file error when the list cannot be saved; the host is then still registered and
    /// nothing else changed. [`Error::CredentialRemains`] when the host is gone but its
    /// credential could not be deleted; calling again retries the deletion.
    pub async fn remove_host(&self, host_id: &HostId) -> Result<(), Error> {
        self.registry.remove(host_id.clone()).await?;
        if self.removed.send(host_id.clone()).is_err() {
            tracing::debug!("no relay is following host removals");
        }
        credential_io::forget(&self.tokens, host_id)
            .await
            .map_err(|source| Error::CredentialRemains {
                host: host_id.clone(),
                source: Box::new(source),
            })
    }

    pub(crate) fn removed_hosts(&self) -> broadcast::Receiver<HostId> {
        self.removed.subscribe()
    }

    /// Answers one `crowbar://<hostId>/v0/...` request made by a pane of `origin`. The answer is
    /// always a response: a failure is a marked proxy answer the page can read.
    pub async fn handle(
        &self,
        origin: &PaneOrigin,
        request: Request<Vec<u8>>,
    ) -> Response<Vec<u8>> {
        let page_origin = request.headers().get(header::ORIGIN).cloned();
        let response = match tokio::time::timeout(PROXY_TIMEOUT, self.route(origin, request)).await
        {
            Ok(response) => response,
            Err(_) => error_response(504, "crowbar daemon did not answer in time"),
        };
        readable_by(page_origin, response)
    }

    async fn route(&self, origin: &PaneOrigin, mut request: Request<Vec<u8>>) -> Response<Vec<u8>> {
        let target_text = request.uri().path_and_query().map_or_else(
            || request.uri().path().to_owned(),
            |pq| pq.as_str().to_owned(),
        );
        if access_scope::check(origin, &target_text).is_err() {
            return error_response(403, "crowbar api is not available to this origin");
        }
        let target = match target_of(request.uri()) {
            Ok(target) => target,
            Err(e) => return error_response(400, &e.to_string()),
        };
        strip_pane_headers(request.headers_mut());
        access_scope::stamp(request.headers_mut(), origin);
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
        request: &UpstreamRequest,
    ) -> Result<UpstreamSocket, Error> {
        upstream::open(self, request).await
    }
}

fn target_of(uri: &http::Uri) -> Result<HostTarget, Error> {
    let host = uri.host().unwrap_or_default();
    let bare = uri
        .authority()
        .is_none_or(|authority| authority.as_str() == host);
    if !bare {
        return Err(Error::InvalidRequest(
            "the authority holds more than a host",
        ));
    }
    match host {
        "" | "localhost" => Ok(HostTarget::Local),
        other => Ok(HostTarget::of(&HostId::new(other)?)),
    }
}

#[cfg(test)]
#[path = "transport_tests.rs"]
mod tests;
