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
    let extension = AccessScope::Extension(protocol::ExtensionId::new("acme").unwrap());
    let answer = transport
        .handle(&extension, get("crowbar://localhost/v0/projects"))
        .await;
    assert_eq!(answer.status(), 403);
    let answer = transport
        .handle(&AccessScope::Core, get("crowbar://localhost/debug/pprof"))
        .await;
    assert_eq!(answer.status(), 403);
}

#[tokio::test]
async fn a_malformed_authority_is_a_bad_request_and_an_unknown_host_a_bad_gateway() {
    let transport = transport(std::env::temp_dir().join("cb-none.sock"));
    let bad = transport
        .handle(&AccessScope::Core, get("crowbar://Host_1/v0/x"))
        .await;
    assert_eq!(bad.status(), 400);
    let unknown = transport
        .handle(&AccessScope::Core, get("crowbar://host-9/v0/x"))
        .await;
    assert_eq!(unknown.status(), 502);
    assert_eq!(marker_of(&unknown), None);
}

#[tokio::test]
async fn removing_a_host_is_announced_to_stream_holders() {
    let transport = transport(std::env::temp_dir().join("cb-none.sock"));
    let mut removed = transport.removed_hosts();
    let id = HostId::new("host-1").unwrap();
    transport.remove_host(&id).unwrap();
    assert_eq!(removed.recv().await.unwrap(), id);
}

#[test]
fn a_page_cannot_forge_the_extension_header() {
    let mut headers = http::HeaderMap::new();
    headers.insert(
        HeaderName::from_static(EXTENSION_HEADER),
        HeaderValue::from_static("victim"),
    );
    stamp_scope(&mut headers, &AccessScope::Core);
    assert!(headers.get(EXTENSION_HEADER).is_none());
    let own = AccessScope::Extension(protocol::ExtensionId::new("acme").unwrap());
    stamp_scope(&mut headers, &own);
    assert_eq!(headers.get(EXTENSION_HEADER).unwrap(), "acme");
}
