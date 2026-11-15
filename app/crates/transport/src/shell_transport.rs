use std::sync::Arc;

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, HostId, RelayEndpoint, RelayScope, Responder,
};
use tokio::runtime::Handle;

use crate::daemon_wire::{from_http, refusal, to_http};
use crate::error::Error;
use crate::host_limits::HostLimits;
use crate::host_target::HostTarget;
use crate::reply::Reply;
use crate::stream_access::StreamAccess;
use crate::transport::{PROXY_TIMEOUT, Transport};

/// The [`DaemonTransport`] the shell holds: HTTP through [`Transport`], WebSocket access through
/// a [`StreamAccess`] (the loopback relay today).
///
/// A request is answered once, from the runtime given to [`ShellTransport::new`]; if that
/// runtime stops first the pane gets `Interrupted`. At most 32 requests per host are in flight,
/// and the rest wait their turn within the proxy timeout.
pub struct ShellTransport {
    runtime: Handle,
    transport: Arc<Transport>,
    access: Arc<dyn StreamAccess>,
    limits: Arc<HostLimits>,
}

impl std::fmt::Debug for ShellTransport {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("ShellTransport").finish_non_exhaustive()
    }
}

impl ShellTransport {
    /// Serves requests on `runtime`, which the shell keeps alive for the app's life.
    #[must_use]
    pub fn new(
        runtime: Handle,
        transport: Arc<Transport>,
        access: Arc<dyn StreamAccess>,
    ) -> Self {
        Self {
            runtime,
            transport,
            access,
            limits: Arc::new(HostLimits::default()),
        }
    }
}

pub(crate) fn into_protocol(error: Error) -> protocol::Error {
    match error {
        Error::UnknownHost(id) => protocol::Error::UnknownHost(id.as_str().to_owned()),
        other => protocol::Error::HostUnavailable(Box::new(other)),
    }
}

async fn answer(
    transport: &Transport,
    limits: &HostLimits,
    request: DaemonRequest,
) -> DaemonResponse {
    let host: HostId = request.host().clone();
    let queued = tokio::time::timeout(PROXY_TIMEOUT, limits.acquire(&host)).await;
    let Ok(Some(_slot)) = queued else {
        return from_http(crate::response::error_response(
            http::StatusCode::SERVICE_UNAVAILABLE,
            "too many requests are waiting for this host",
        ));
    };
    match to_http(request) {
        Ok((origin, http_request)) => from_http(transport.handle(&origin, http_request).await),
        Err(e) => refusal(&e),
    }
}

impl DaemonTransport for ShellTransport {
    fn request(
        &self,
        request: DaemonRequest,
        respond: Responder,
    ) {
        let reply = Reply::new(respond);
        let transport = Arc::clone(&self.transport);
        let limits = Arc::clone(&self.limits);
        self.runtime.spawn(async move {
            reply.send(answer(&transport, &limits, request).await);
        });
    }

    fn relay_endpoint(
        &self,
        scope: &RelayScope,
    ) -> protocol::Result<RelayEndpoint> {
        if let HostTarget::Remote(id) = HostTarget::of(scope.host())
            && self.transport.registry().get(&id).is_none()
        {
            return Err(protocol::Error::UnknownHost(id.as_str().to_owned()));
        }
        self.access.issue(scope).map_err(into_protocol)
    }

    fn revoke_pane(
        &self,
        scope: &RelayScope,
    ) {
        self.access.revoke_pane(scope.pane());
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;

    use protocol::{DaemonResponse, ExtensionId, HttpMethod, PaneId, PaneOrigin, RequestTarget};

    use super::*;
    use crate::host_registry::HostRegistry;
    use crate::memory_token_store::MemoryTokenStore;

    #[derive(Default)]
    struct RecordingAccess {
        issued: Mutex<Vec<RelayScope>>,
        revoked: Mutex<Vec<PaneId>>,
    }

    impl StreamAccess for RecordingAccess {
        fn issue(
            &self,
            scope: &RelayScope,
        ) -> Result<RelayEndpoint, Error> {
            self.issued.lock().unwrap().push(scope.clone());
            Ok(RelayEndpoint::new("ws://127.0.0.1:1", "t"))
        }

        fn revoke_pane(
            &self,
            pane: &PaneId,
        ) {
            self.revoked.lock().unwrap().push(pane.clone());
        }
    }

    fn shell() -> (ShellTransport, Arc<RecordingAccess>) {
        let transport = Transport::new(
            Arc::new(std::env::temp_dir().join("cb-shell-none.sock")),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        );
        let access = Arc::new(RecordingAccess::default());
        (
            ShellTransport::new(
                Handle::current(),
                Arc::new(transport),
                Arc::clone(&access) as Arc<dyn StreamAccess>,
            ),
            access,
        )
    }

    fn scope(host: &str) -> RelayScope {
        RelayScope::new(
            HostId::new(host).unwrap(),
            PaneId::new("p1").unwrap(),
            PaneOrigin::Core,
        )
    }

    fn request(
        path: &str,
        origin: PaneOrigin,
    ) -> DaemonRequest {
        DaemonRequest::new(
            HostId::new("local").unwrap(),
            origin,
            HttpMethod::new("GET").unwrap(),
            RequestTarget::new(path).unwrap(),
        )
    }

    async fn answer_of(
        shell: &ShellTransport,
        request: DaemonRequest,
    ) -> DaemonResponse {
        let (sender, receiver) = tokio::sync::oneshot::channel();
        shell.request(
            request,
            Box::new(move |result| {
                sender.send(result).ok();
            }),
        );
        receiver.await.unwrap().unwrap()
    }

    #[tokio::test]
    async fn a_request_outside_the_origins_scope_is_answered_403_without_a_daemon() {
        let (shell, _) = shell();
        let acme = PaneOrigin::Extension(ExtensionId::new("acme").unwrap());
        let answer = answer_of(&shell, request("/v0/projects", acme)).await;
        assert_eq!(answer.status(), 403);
    }

    fn forward_to(
        sender: tokio::sync::oneshot::Sender<Result<DaemonResponse, protocol::Error>>
    ) -> Responder {
        Box::new(move |result| {
            sender.send(result).ok();
        })
    }

    #[tokio::test]
    async fn a_reply_the_runtime_never_ran_is_answered_interrupted() {
        let (sender, receiver) = tokio::sync::oneshot::channel();
        let respond = forward_to(sender);
        drop(Reply::new(respond));
        assert!(matches!(
            receiver.await.unwrap(),
            Err(protocol::Error::Interrupted)
        ));
    }

    #[tokio::test]
    async fn a_relay_endpoint_is_issued_for_the_local_host_but_not_an_unregistered_one() {
        let (shell, access) = shell();
        assert_eq!(shell.relay_endpoint(&scope("local")).unwrap().token(), "t");
        assert_eq!(access.issued.lock().unwrap().len(), 1);
        let error = shell.relay_endpoint(&scope("host-9")).unwrap_err();
        assert!(matches!(error, protocol::Error::UnknownHost(_)), "{error}");
        assert_eq!(
            access.issued.lock().unwrap().len(),
            1,
            "nothing is issued for an unknown host"
        );
    }

    type Kind = fn(&protocol::Error) -> bool;

    fn cases() -> [(Error, Kind); 2] {
        [
            (
                Error::UnknownHost(HostId::new("host-1").unwrap()),
                |mapped| matches!(mapped, protocol::Error::UnknownHost(_)),
            ),
            (Error::Forbidden("x".into()), |mapped| {
                matches!(mapped, protocol::Error::HostUnavailable(_))
            }),
        ]
    }

    #[tokio::test]
    async fn revoking_a_pane_scope_revokes_that_pane_in_the_stream_access() {
        let (shell, access) = shell();
        shell.revoke_pane(&scope("local"));
        assert_eq!(*access.revoked.lock().unwrap(), [PaneId::new("p1").unwrap()]);
    }

    #[test]
    fn errors_map_to_the_protocol_kinds() {
        for (error, is_expected) in cases() {
            let described = error.to_string();
            assert!(is_expected(&into_protocol(error)), "{described}");
        }
    }
}
