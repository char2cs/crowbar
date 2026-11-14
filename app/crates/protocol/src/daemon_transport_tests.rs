use super::*;
use crate::error::Error;
use crate::host_id::HostId;
use crate::pane_kind::PaneKind;

struct Refusing;

impl DaemonTransport for Refusing {
    fn request(&self, _request: DaemonRequest, respond: Responder) {
        respond(Err(Error::Denied("no".into())));
    }

    fn relay_endpoint(&self, _scope: &RelayScope) -> Result<RelayEndpoint> {
        Err(Error::Timeout)
    }
}

#[test]
fn a_transport_is_usable_as_a_shared_trait_object() {
    let transport: std::sync::Arc<dyn DaemonTransport> = std::sync::Arc::new(Refusing);
    let request = DaemonRequest {
        host: HostId::new("local").unwrap(),
        origin: PaneKind::Core,
        extension: None,
        method: "GET".into(),
        path_and_query: "/v0/health".into(),
        headers: Vec::new(),
        body: Vec::new(),
    };
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
