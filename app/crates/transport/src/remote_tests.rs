use super::*;
use crate::memory_token_store::MemoryTokenStore;

#[test]
fn a_bearer_header_is_sensitive_and_refuses_newlines() {
    assert!(bearer("abc").unwrap().is_sensitive());
    assert!(matches!(bearer("a\nb"), Err(Error::BadToken)));
}

#[tokio::test]
async fn a_host_with_no_registration_or_credential_is_never_dialled() {
    let registry = HostRegistry::in_memory();
    let tokens: Arc<dyn TokenStore> = Arc::new(MemoryTokenStore::default());
    let id = HostId::new("host-1").unwrap();
    assert!(matches!(
        paired_host(&registry, &tokens, &id).await,
        Err(Error::UnknownHost(_))
    ));
    registry
        .insert_blocking(
            id.clone(),
            crate::endpoint::Endpoint::parse("10.0.0.2:7000").unwrap(),
        )
        .unwrap();
    assert!(matches!(
        paired_host(&registry, &tokens, &id).await,
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

#[test]
fn a_missing_credential_or_host_is_marked_so_the_page_can_offer_pairing() {
    let none = credential_response(&Error::NoCredential);
    assert_eq!(none.status(), 401);
    assert_eq!(
        none.headers().get("x-crowbar-proxy").unwrap(),
        "no-credential"
    );
    let unknown = credential_response(&Error::UnknownHost(HostId::new("host-1").unwrap()));
    assert_eq!(unknown.status(), 502);
    assert_eq!(
        unknown.headers().get("x-crowbar-proxy").unwrap(),
        "unknown-host"
    );
}
