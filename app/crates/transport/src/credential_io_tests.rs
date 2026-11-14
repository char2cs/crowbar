use crate::memory_token_store::MemoryTokenStore;

use super::*;

fn host() -> HostId {
    HostId::new("host-1").unwrap()
}

#[tokio::test]
async fn a_kept_credential_loads_and_a_forgotten_one_is_gone() {
    let store = Arc::new(MemoryTokenStore::default());
    let tokens = Arc::clone(&store) as Arc<dyn TokenStore>;
    let credential = Credential::new("t".into(), [1; 32]);
    keep(&tokens, &host(), &credential).await.unwrap();
    assert_eq!(load(&tokens, &host()).await.unwrap(), credential);
    forget(&tokens, &host()).await.unwrap();
    assert!(matches!(
        load(&tokens, &host()).await,
        Err(Error::NoCredential)
    ));
    assert_eq!(
        store.get("host-1").unwrap(),
        None,
        "nothing left in the store"
    );
}

#[tokio::test]
async fn an_entry_pairing_did_not_write_is_no_credential() {
    let store = MemoryTokenStore::default();
    store.set("host-1", "sometoken").unwrap();
    let tokens: Arc<dyn TokenStore> = Arc::new(store);
    assert!(matches!(
        load(&tokens, &host()).await,
        Err(Error::NoCredential)
    ));
    let other = HostId::new("host-2").unwrap();
    assert!(matches!(
        load(&tokens, &other).await,
        Err(Error::NoCredential)
    ));
}
