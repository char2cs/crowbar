//! The agent descriptors a computer loads, and installing another.

use crate::call::Call;
use crate::daemon_client::DaemonClient;
use crate::descriptor_report::DescriptorReport;
use crate::error::Result;
use crate::typed::typed;

pub(crate) const PATH: &str = "/v0/settings/chat/descriptors";

pub(crate) async fn list(client: &DaemonClient) -> Result<Vec<DescriptorReport>> {
    let data = client.send(Call::get(PATH)).await?;
    typed(PATH, data)
}

pub(crate) async fn upload(
    client: &DaemonClient,
    text: Vec<u8>,
) -> Result<DescriptorReport> {
    let data = client.send(Call::post(PATH).yaml(text)).await?;
    typed(PATH, data)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[tokio::test]
    async fn descriptors_are_listed_and_an_upload_posts_the_yaml_as_the_body() {
        let (state, daemon) = state();
        let client = state.hosts[0].client.clone();
        daemon.ok("GET", PATH, r#"[{"id":"claude"}]"#);
        daemon.ok("POST", PATH, r#"{"id":"mine","findings":[]}"#);
        assert_eq!(list(&client).await.unwrap().len(), 1);
        let report = upload(&client, b"id: mine\n".to_vec()).await.unwrap();
        assert_eq!(report.id, "mine");
        let seen = daemon.seen();
        let posted = seen.iter().find(|call| call.method == "POST").unwrap();
        assert!(
            posted
                .headers
                .iter()
                .any(|(name, value)| name == "content-type" && value == "application/yaml")
        );
    }

    #[tokio::test]
    async fn a_null_answer_lists_no_descriptors() {
        let (state, daemon) = state();
        let client = state.hosts[0].client.clone();
        daemon.ok("GET", PATH, "null");
        assert_eq!(list(&client).await.unwrap().len(), 0);
    }
}
