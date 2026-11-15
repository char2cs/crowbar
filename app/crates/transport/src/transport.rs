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
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
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
    pub async fn remove_host(
        &self,
        host_id: &HostId,
    ) -> Result<(), Error> {
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

    async fn route(
        &self,
        origin: &PaneOrigin,
        mut request: Request<Vec<u8>>,
    ) -> Response<Vec<u8>> {
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

    async fn forward_to_local(
        &self,
        request: Request<Vec<u8>>,
    ) -> Response<Vec<u8>> {
        forward_local(self.locator.as_ref(), request, |attempt| {
            tokio::time::sleep(connect_retry_delay(attempt))
        })
        .await
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
mod tests {
    use super::*;
    use crate::memory_token_store::MemoryTokenStore;
    use crate::response::marker_of;

    fn transport(socket: std::path::PathBuf) -> Transport {
        Transport::new(
            Arc::new(socket),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        )
    }

    fn get(uri: &str) -> Request<Vec<u8>> {
        Request::builder().uri(uri).body(Vec::new()).unwrap()
    }

    #[tokio::test]
    async fn a_path_outside_the_scope_never_reaches_a_daemon() {
        let transport = transport(std::env::temp_dir().join("cb-none.sock"));
        let extension = PaneOrigin::Extension(protocol::ExtensionId::new("acme").unwrap());
        let answer = transport
            .handle(&extension, get("crowbar://localhost/v0/projects"))
            .await;
        assert_eq!(answer.status(), 403);
        let answer = transport
            .handle(&PaneOrigin::Core, get("crowbar://localhost/debug/pprof"))
            .await;
        assert_eq!(answer.status(), 403);
    }

    #[tokio::test]
    async fn a_malformed_authority_is_a_bad_request_and_an_unknown_host_a_bad_gateway() {
        let transport = transport(std::env::temp_dir().join("cb-none.sock"));
        let bad = transport
            .handle(&PaneOrigin::Core, get("crowbar://Host_1/v0/x"))
            .await;
        assert_eq!(bad.status(), 400);
        let unknown = transport
            .handle(&PaneOrigin::Core, get("crowbar://host-9/v0/x"))
            .await;
        assert_eq!(unknown.status(), 502);
        assert_eq!(marker_of(&unknown), Some(crate::response::UNKNOWN_HOST));
    }

    #[tokio::test]
    async fn removing_a_host_is_announced_to_stream_holders_and_forgets_its_credential() {
        let tokens = Arc::new(MemoryTokenStore::default());
        tokens.set("host-1", "entry").unwrap();
        let transport = Transport::new(
            Arc::new(std::env::temp_dir().join("cb-none.sock")),
            Arc::new(HostRegistry::in_memory()),
            Arc::clone(&tokens) as Arc<dyn TokenStore>,
        );
        let mut removed = transport.removed_hosts();
        let id = HostId::new("host-1").unwrap();
        transport.remove_host(&id).await.unwrap();
        assert_eq!(removed.recv().await.unwrap(), id);
        assert_eq!(
            tokens.get("host-1").unwrap(),
            None,
            "the keychain entry is deleted"
        );
    }

    struct StuckStore;

    impl TokenStore for StuckStore {
        fn get(
            &self,
            _: &str,
        ) -> Result<Option<String>, Error> {
            Ok(None)
        }

        fn set(
            &self,
            _: &str,
            _: &str,
        ) -> Result<(), Error> {
            Ok(())
        }

        fn delete(
            &self,
            _: &str,
        ) -> Result<(), Error> {
            Err(Error::Store("keychain locked".into()))
        }
    }

    #[tokio::test]
    async fn a_credential_that_cannot_be_deleted_is_reported_but_the_host_is_already_unreachable() {
        let registry = Arc::new(HostRegistry::in_memory());
        let id = HostId::new("host-1").unwrap();
        registry
            .insert(
                id.clone(),
                crate::endpoint::Endpoint::parse("10.0.0.2:1").unwrap(),
            )
            .await
            .unwrap();
        let transport = Transport::new(
            Arc::new(std::env::temp_dir().join("cb-none.sock")),
            Arc::clone(&registry),
            Arc::new(StuckStore),
        );
        let mut removed = transport.removed_hosts();
        let error = transport.remove_host(&id).await.unwrap_err();
        assert!(matches!(error, Error::CredentialRemains { .. }), "{error}");
        assert!(registry.get(&id).is_none(), "the host is gone");
        assert_eq!(
            removed.recv().await.unwrap(),
            id,
            "and its streams were told to end"
        );
    }

    #[test]
    fn an_authority_that_holds_more_than_a_host_is_refused() {
        for uri in [
            "crowbar://user@host-1/v0/x",
            "crowbar://localhost@host-2/v0/x",
            "crowbar://host-1:99/v0/x",
        ] {
            let uri: http::Uri = uri.parse().unwrap();
            assert!(target_of(&uri).is_err(), "{uri}");
        }
        let local: http::Uri = "crowbar://localhost/v0/x".parse().unwrap();
        assert_eq!(target_of(&local).unwrap(), HostTarget::Local);
    }
}
