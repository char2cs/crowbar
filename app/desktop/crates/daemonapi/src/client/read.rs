use serde::de::DeserializeOwned;

use crate::client::Client;
use crate::client::call::Call;
use crate::client::decode::typed;
use crate::error::Result;

impl Client {
    pub async fn read<T: DeserializeOwned>(
        &self,
        path: impl Into<String>,
    ) -> Result<T> {
        self.fetch(Call::get(path)).await
    }

    pub async fn fetch<T: DeserializeOwned>(
        &self,
        call: Call,
    ) -> Result<T> {
        let path = call.path.clone();
        let data = self.send(call).await?;
        typed(&path, data)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use protocol::HostId;

    use super::*;
    use crate::test_support::FakeDaemon;

    async fn readable(body: &str) -> bool {
        let daemon = FakeDaemon::shared();
        let client = Client::new(
            Arc::<FakeDaemon>::clone(&daemon),
            HostId::new("local").unwrap(),
        );
        daemon.ok("GET", "/v0/route", body);
        client.read::<Vec<String>>("/v0/route").await.is_ok()
    }

    #[tokio::test]
    async fn an_answer_is_read_as_the_promised_type_or_refused() {
        assert!(readable(r#"["a","b"]"#).await, "a list is a list");
        assert!(!readable(r#"{"not":"a list"}"#).await, "an object is not");
    }
}
