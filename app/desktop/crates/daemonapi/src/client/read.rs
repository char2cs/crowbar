//! Reading a daemon route as the type it promises.

use serde::de::DeserializeOwned;

use crate::client::Client;
use crate::client::call::Call;
use crate::client::decode::typed;
use crate::error::Result;

/// Gets `path` through `client` and returns the answer's `data` as the type the route promises.
///
/// # Errors
///
/// `Error::Request`, `Error::Refused` or `Error::Body` when the request fails, as in
/// [`Client::send`], and `Error::Json` when the `data` is not a `T`.
pub async fn read<T: DeserializeOwned>(
    client: &Client,
    path: String,
) -> Result<T> {
    let data = client.send(Call::get(path.clone())).await?;
    typed(&path, data)
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
        read::<Vec<String>>(&client, "/v0/route".into())
            .await
            .is_ok()
    }

    #[tokio::test]
    async fn an_answer_is_read_as_the_promised_type_or_refused() {
        assert!(readable(r#"["a","b"]"#).await, "a list is a list");
        assert!(!readable(r#"{"not":"a list"}"#).await, "an object is not");
    }
}
