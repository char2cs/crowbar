use std::sync::Arc;

use protocol::HostId;

use crate::blocking::blocking;
use crate::credential::Credential;
use crate::error::Error;
use crate::token_store::TokenStore;

pub(crate) async fn load(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
) -> Result<Credential, Error> {
    let (store, key) = (Arc::clone(tokens), host_id.to_string());
    let entry = blocking(move || store.get(&key)).await?;
    entry
        .as_deref()
        .and_then(Credential::decode)
        .ok_or(Error::NoCredential)
}

pub(crate) async fn keep(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
    credential: &Credential,
) -> Result<(), Error> {
    let (store, key) = (Arc::clone(tokens), host_id.to_string());
    let entry = credential.encode();
    blocking(move || store.set(&key, &entry)).await
}

pub(crate) async fn forget(tokens: &Arc<dyn TokenStore>, host_id: &HostId) -> Result<(), Error> {
    let (store, key) = (Arc::clone(tokens), host_id.to_string());
    blocking(move || store.delete(&key)).await
}

#[cfg(test)]
#[path = "credential_io_tests.rs"]
mod tests;
