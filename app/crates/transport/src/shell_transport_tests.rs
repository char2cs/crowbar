use std::sync::Mutex;

use protocol::{DaemonResponse, ExtensionId, HostId, PaneId, PaneKind};

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
        Ok(RelayEndpoint {
            url: "ws://127.0.0.1:1".into(),
            token: "t".into(),
        })
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
    RelayScope {
        host: HostId::new(host).unwrap(),
        pane: PaneId::new("p1").unwrap(),
        kind: PaneKind::Core,
        extension: None,
    }
}

fn request(path: &str, kind: PaneKind, extension: Option<&str>) -> DaemonRequest {
    DaemonRequest {
        host: HostId::new("local").unwrap(),
        origin: kind,
        extension: extension.map(|id| ExtensionId::new(id).unwrap()),
        method: "GET".into(),
        path_and_query: path.into(),
        headers: Vec::new(),
        body: Vec::new(),
    }
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
    let answer = answer_of(
        &shell,
        request("/v0/projects", PaneKind::Extension, Some("acme")),
    )
    .await;
    assert_eq!(answer.status, 403);
}

#[tokio::test]
async fn an_extension_origin_without_its_id_is_refused() {
    let (shell, _) = shell();
    let answer = answer_of(&shell, request("/v0/ext/acme/x", PaneKind::Extension, None)).await;
    assert_eq!(answer.status, 403);
}

#[tokio::test]
async fn a_relay_endpoint_is_issued_for_the_local_host_but_not_an_unregistered_one() {
    let (shell, access) = shell();
    assert_eq!(shell.relay_endpoint(&scope("local")).unwrap().token, "t");
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
        into_protocol(Error::BadGrant("x")),
        protocol::Error::Denied(_)
    ));
    assert!(matches!(
        into_protocol(Error::Forbidden("x".into())),
        protocol::Error::HostUnavailable(_)
    ));
}
