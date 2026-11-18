use crate::error::Result;
use crate::port::daemon_request::DaemonRequest;
use crate::port::daemon_response::DaemonResponse;
use crate::port::relay_endpoint::RelayEndpoint;
use crate::port::relay_scope::RelayScope;

/// Completion callback of [`DaemonTransport::request`]; called exactly once.
pub type Responder = Box<dyn FnOnce(Result<DaemonResponse>) + Send + 'static>;

/// How the shell reaches daemon hosts.
///
/// The shell serves `crowbar://<host>/...` requests and hands out WebSocket relay endpoints
/// through this trait. The implementation (unix socket, paired remote host, token and TLS pin
/// handling) lives in the `transport` crate. The trait is callback based so it needs no async
/// runtime: an implementation may answer from any thread.
///
/// Only a pane has a [`PaneOrigin`](crate::PaneOrigin), so a browser tab, which has none, can
/// neither build a [`DaemonRequest`] nor ask for a [`RelayScope`]; the shell must not invent one
/// for it.
pub trait DaemonTransport: Send + Sync + 'static {
    /// Forwards `request` and calls `respond` exactly once, from any thread, with
    /// [`Error::Interrupted`](crate::Error::Interrupted) if the transport stops first. Must not
    /// block the caller.
    ///
    /// The platform's custom protocol task (WebKit's URL scheme task) may only be completed on
    /// the UI thread, so the `respond` the shell passes in hops there; an implementation never
    /// assumes it runs on the caller's thread.
    fn request(
        &self,
        request: DaemonRequest,
        respond: Responder,
    );

    /// Issues a relay endpoint and token for a pane's WebSocket streams.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the host is unknown or unreachable.
    fn relay_endpoint(
        &self,
        scope: &RelayScope,
    ) -> Result<RelayEndpoint>;

    /// Revokes the relay token issued for `scope`'s pane and closes its open streams, so a token
    /// does not outlive the pane. A transport that issues no tokens has nothing to revoke.
    fn revoke_pane(
        &self,
        _scope: &RelayScope,
    ) {
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::error::Error;
    use crate::identity::HostId;
    use crate::identity::PaneOrigin;
    use crate::port::http_method::HttpMethod;
    use crate::port::request_target::RequestTarget;

    struct Refusing;

    impl DaemonTransport for Refusing {
        fn request(
            &self,
            _request: DaemonRequest,
            respond: Responder,
        ) {
            respond(Err(Error::Denied("no")));
        }

        fn relay_endpoint(
            &self,
            _scope: &RelayScope,
        ) -> Result<RelayEndpoint> {
            Err(Error::Timeout)
        }
    }

    #[test]
    fn a_transport_that_issues_no_tokens_has_nothing_to_revoke() {
        let transport: std::sync::Arc<dyn DaemonTransport> = std::sync::Arc::new(Refusing);
        let scope = RelayScope::new(
            HostId::new("local").unwrap(),
            crate::identity::PaneId::new("p1").unwrap(),
            PaneOrigin::Core,
        );
        transport.revoke_pane(&scope);
        assert!(transport.relay_endpoint(&scope).is_err(), "still refusing");
    }

    #[test]
    fn a_transport_is_usable_as_a_shared_trait_object() {
        let transport: std::sync::Arc<dyn DaemonTransport> = std::sync::Arc::new(Refusing);
        let request = DaemonRequest::new(
            HostId::new("local").unwrap(),
            PaneOrigin::Core,
            HttpMethod::new("GET").unwrap(),
            RequestTarget::new("/v0/health").unwrap(),
        );
        let (sender, receiver) = std::sync::mpsc::channel();
        transport.request(
            request,
            Box::new(move |result| sender.send(result).unwrap()),
        );
        assert!(
            matches!(receiver.recv().unwrap(), Err(Error::Denied(_))),
            "responder called once"
        );
    }
}
