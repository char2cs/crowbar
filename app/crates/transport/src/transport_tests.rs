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
    fn get(&self, _: &str) -> Result<Option<String>, Error> {
        Ok(None)
    }

    fn set(&self, _: &str, _: &str) -> Result<(), Error> {
        Ok(())
    }

    fn delete(&self, _: &str) -> Result<(), Error> {
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
