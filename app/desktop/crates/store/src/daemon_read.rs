//! Reading a daemon route as the type it promises.

use serde::de::DeserializeOwned;

use crate::call::Call;
use crate::daemon_client::DaemonClient;
use crate::error::Result;
use crate::typed::typed;

pub(crate) async fn read<T: DeserializeOwned>(
    client: &DaemonClient,
    path: String,
) -> Result<T> {
    let data = client.send(Call::get(path.clone())).await?;
    typed(&path, data)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    async fn readable(body: &str) -> bool {
        let (state, daemon) = state();
        let client = state.hosts[0].client.clone();
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
