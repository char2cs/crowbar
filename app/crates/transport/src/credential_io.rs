use std::sync::Arc;

use protocol::HostId;

use crate::blocking::run_blocking;
use crate::credential::Credential;
use crate::error::Error;
use crate::token_store::TokenStore;

pub(crate) async fn load(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
) -> Result<Credential, Error> {
    let (store, key) = (Arc::clone(tokens), host_id.to_string());
    let entry = run_blocking(move || store.load_entry(&key)).await?;
    entry
        .as_deref()
        .and_then(Credential::decode)
        .ok_or(Error::NoCredential)
}

pub(crate) async fn persist_credential(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
    credential: &Credential,
) -> Result<(), Error> {
    let (store, key) = (Arc::clone(tokens), host_id.to_string());
    let entry = credential.encode();
    run_blocking(move || store.set(&key, &entry)).await
}

pub(crate) async fn forget(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
) -> Result<(), Error> {
    let (store, key) = (Arc::clone(tokens), host_id.to_string());
    run_blocking(move || store.delete(&key)).await
}

#[cfg(test)]
mod tests {
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
        persist_credential(&tokens, &host(), &credential).await.unwrap();
        assert_eq!(load(&tokens, &host()).await.unwrap(), credential);
        forget(&tokens, &host()).await.unwrap();
        assert!(matches!(
            load(&tokens, &host()).await,
            Err(Error::NoCredential)
        ));
        assert_eq!(
            store.load_entry("host-1").unwrap(),
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
}
