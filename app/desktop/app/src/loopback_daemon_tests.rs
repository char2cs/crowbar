use std::sync::mpsc;

use protocol::{HostId, PaneKind};

use super::*;

fn ask(method: &str, path: &str) -> DaemonResponse {
    let (sender, receiver) = mpsc::channel();
    let request = DaemonRequest {
        host: HostId::new("local").unwrap(),
        origin: PaneKind::Core,
        extension: None,
        method: method.into(),
        path_and_query: path.into(),
        headers: Vec::new(),
        body: Vec::new(),
    };
    LoopbackDaemon.request(request, Box::new(move |result| sender.send(result).unwrap()));
    receiver.recv().unwrap().unwrap()
}

#[test]
fn answers_health_and_nothing_else() {
    assert_eq!(ask("GET", "/v0/health").status, 200, "health");
    assert_eq!(ask("GET", "/v0/threads").status, 404, "other path");
    assert_eq!(ask("POST", "/v0/health").status, 404, "other method");
}

#[test]
fn has_no_relay() {
    let scope = RelayScope {
        host: HostId::new("local").unwrap(),
        pane: protocol::PaneId::new("p").unwrap(),
        kind: PaneKind::Core,
        extension: None,
    };
    assert!(LoopbackDaemon.relay_endpoint(&scope).is_err(), "no relay");
}
