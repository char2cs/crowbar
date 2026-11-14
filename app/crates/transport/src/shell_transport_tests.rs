use std::sync::Mutex;

use protocol::{DaemonResponse, ExtensionId, HttpMethod, PaneId, PaneOrigin, RequestTarget};

use super::*;
use crate::host_registry::HostRegistry;
use crate::memory_token_store::MemoryTokenStore;

#[derive(Default)]
struct RecordingAccess {
    issued: Mutex<Vec<RelayScope>>,
}

impl StreamAccess for RecordingAccess {
    fn issue(&self, scope: &RelayScope) -> Result<RelayEndpoint, Error> {
        self.issued.lock().unwrap().push(scope.clone());
        Ok(RelayEndpoint::new("ws://127.0.0.1:1", "t"))
    }

    fn revoke_pane(&self, _pane: &PaneId) {}
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

fn request(path: &str, origin: PaneOrigin) -> DaemonRequest {
    DaemonRequest::new(
        HostId::new("local").unwrap(),
        origin,
        HttpMethod::new("GET").unwrap(),
        RequestTarget::new(path).unwrap(),
    )
}

async fn answer_of(shell: &ShellTransport, request: DaemonRequest) -> DaemonResponse {
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

#[tokio::test]
async fn a_reply_the_runtime_never_ran_is_answered_interrupted() {
    let (sender, receiver) = tokio::sync::oneshot::channel();
    let respond: Responder = Box::new(move |result| {
        sender.send(result).ok();
    });
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

#[test]
fn errors_map_to_the_protocol_kinds() {
    let id = HostId::new("host-1").unwrap();
    assert!(matches!(
        into_protocol(Error::UnknownHost(id)),
        protocol::Error::UnknownHost(_)
    ));
    assert!(matches!(
        into_protocol(Error::Forbidden("x".into())),
        protocol::Error::HostUnavailable(_)
    ));
}
