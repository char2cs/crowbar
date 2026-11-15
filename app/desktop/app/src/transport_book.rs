//! The transport's registry and token store, seen as the host book pairing works against.

use std::sync::Arc;

use protocol::HostId;
use shell_data::{BookFuture, HostBook, PairingRequest};
use transport::{HostRegistry, PairRequest, TokenStore};

pub(crate) struct TransportBook {
    registry: Arc<HostRegistry>,
    tokens: Arc<dyn TokenStore>,
}

impl TransportBook {
    pub(crate) fn new(
        registry: Arc<HostRegistry>,
        tokens: Arc<dyn TokenStore>,
    ) -> Self {
        Self { registry, tokens }
    }
}

impl HostBook for TransportBook {
    fn probe(
        &self,
        endpoint: &str,
    ) -> BookFuture<String> {
        let endpoint = endpoint.to_owned();
        Box::pin(async move {
            transport::probe_fingerprint(&endpoint)
                .await
                .map_err(|error| error.to_string())
        })
    }

    fn pair(
        &self,
        request: PairingRequest,
    ) -> BookFuture<String> {
        let (registry, tokens) = (Arc::clone(&self.registry), Arc::clone(&self.tokens));
        Box::pin(async move {
            let wire = PairRequest {
                host_id: &request.host_id,
                endpoint: &request.endpoint,
                fingerprint: &request.fingerprint,
                code: &request.code,
                name: &request.name,
            };
            transport::pair_remote_host(&registry, &tokens, &wire)
                .await
                .map_err(|error| error.to_string())
        })
    }

    fn unpair(
        &self,
        host: &str,
    ) -> BookFuture<()> {
        let (registry, tokens) = (Arc::clone(&self.registry), Arc::clone(&self.tokens));
        let host = host.to_owned();
        Box::pin(async move {
            let id = HostId::new(&host).map_err(|error| error.to_string())?;
            registry
                .remove(id)
                .await
                .map_err(|error| error.to_string())?;
            tokens.delete(&host).map_err(|error| error.to_string())
        })
    }
}

#[cfg(test)]
mod tests {
    use transport::MemoryTokenStore;

    use super::*;

    #[tokio::test]
    async fn an_address_nobody_answers_at_is_an_error_text_not_a_fingerprint() {
        let dir = tempfile::tempdir().unwrap();
        let registry = Arc::new(HostRegistry::open(dir.path().join("hosts.json")).unwrap());
        let book = TransportBook::new(registry, Arc::new(MemoryTokenStore::default()));
        assert!(book.probe("127.0.0.1:1").await.is_err());
        assert!(book.unpair("nobody").await.is_ok());
    }
}
