use super::*;
use crate::memory_token_store::MemoryTokenStore;

#[test]
fn a_bearer_header_is_sensitive_and_refuses_newlines() {
    assert!(bearer("abc").unwrap().is_sensitive());
    assert!(matches!(bearer("a\nb"), Err(Error::BadToken)));
}

#[tokio::test]
async fn an_entry_pairing_did_not_write_is_no_credential() {
    let store = MemoryTokenStore::default();
    store.set("host-1", "sometoken").unwrap();
    let tokens: Arc<dyn TokenStore> = Arc::new(store);
    let id = HostId::new("host-1").unwrap();
    assert!(matches!(
        stored_credential(&tokens, &id).await,
        Err(Error::NoCredential)
    ));
    let other = HostId::new("host-2").unwrap();
    assert!(matches!(
        stored_credential(&tokens, &other).await,
        Err(Error::NoCredential)
    ));
}

#[test]
fn a_dial_failure_keeps_unreachable_and_impostor_apart() {
    let outage = dial_response(&Error::HostUnreachable("down".into()));
    let impostor = dial_response(&Error::IdentityChanged("changed".into()));
    assert_eq!(outage.status(), 503);
    assert_eq!(impostor.status(), 502);
    assert_ne!(
        outage.headers().get("x-crowbar-proxy"),
        impostor.headers().get("x-crowbar-proxy")
    );
}
