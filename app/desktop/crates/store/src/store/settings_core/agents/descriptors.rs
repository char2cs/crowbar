//! The agent descriptors a computer loads, and installing another.

use daemonapi::Call;

use crate::error::Result;
use crate::store::settings_core::agents::descriptor_report::DescriptorReport;

pub(crate) const PATH: &str = "/v0/settings/chat/descriptors";

pub(crate) async fn list(client: &daemonapi::Client) -> Result<Vec<DescriptorReport>> {
    Ok(client.read(PATH).await?)
}

pub(crate) async fn upload(
    client: &daemonapi::Client,
    text: Vec<u8>,
) -> Result<DescriptorReport> {
    Ok(client.fetch(Call::post(PATH).yaml(text)).await?)
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
        daemon.ok("GET", PATH, "null");
        assert_eq!(list(&client).await.unwrap().len(), 0, "a null answer lists none");
    }
}
