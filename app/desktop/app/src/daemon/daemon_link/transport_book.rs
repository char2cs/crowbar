use std::sync::Arc;

use protocol::HostId;
use store::{BookError, BookFuture, HostBook, PairingRequest};
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
                .map_err(BookError::new)
        })
    }

    fn pair(
        &self,
        request: PairingRequest,
    ) -> BookFuture<String> {
        let (registry, tokens) = (Arc::clone(&self.registry), Arc::clone(&self.tokens));
        Box::pin(async move {
            transport::pair_remote_host(&registry, &tokens, &wire_of(&request))
                .await
                .map_err(BookError::new)
        })
    }

    fn unpair(
        &self,
        host: &str,
    ) -> BookFuture<()> {
        let (registry, tokens) = (Arc::clone(&self.registry), Arc::clone(&self.tokens));
        let host = host.to_owned();
        Box::pin(async move {
            let id = HostId::new(&host).map_err(BookError::new)?;
            registry.remove(id).await.map_err(BookError::new)?;
            tokens.delete(&host).map_err(BookError::new)
        })
    }
}

fn wire_of(request: &PairingRequest) -> PairRequest<'_> {
    PairRequest {
        host_id: &request.host_id,
        endpoint: &request.endpoint,
        fingerprint: &request.fingerprint,
        code: &request.code,
        name: &request.name,
    }
}

#[cfg(test)]
mod tests {
    use transport::MemoryTokenStore;

    use super::*;

    #[tokio::test]
    async fn an_address_nobody_answers_at_is_an_error_text_not_a_fingerprint() {
        let registry = Arc::new(HostRegistry::in_memory());
        let book = TransportBook::new(registry, Arc::new(MemoryTokenStore::default()));
        assert!(book.probe("127.0.0.1:1").await.is_err());
        assert!(book.unpair("nobody").await.is_ok());
    }
}
