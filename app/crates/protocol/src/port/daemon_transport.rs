use crate::error::Result;
use crate::port::daemon_request::DaemonRequest;
use crate::port::daemon_response::DaemonResponse;
use crate::port::relay_endpoint::RelayEndpoint;
use crate::port::relay_scope::RelayScope;

pub type Responder = Box<dyn FnOnce(Result<DaemonResponse>) + Send + 'static>;

pub trait DaemonTransport: Send + Sync + 'static {
    fn request(
        &self,
        request: DaemonRequest,
        respond: Responder,
    );

    fn relay_endpoint(
        &self,
        scope: &RelayScope,
    ) -> Result<RelayEndpoint>;

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
}
