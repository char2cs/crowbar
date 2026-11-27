use std::fmt;

use serde::{Deserialize, Serialize};

#[derive(Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RelayEndpoint {
    url: String,
    token: String,
}

impl RelayEndpoint {
    #[must_use]
    pub fn new(
        url: impl Into<String>,
        token: impl Into<String>,
    ) -> Self {
        Self {
            url: url.into(),
            token: token.into(),
        }
    }

    #[must_use]
    pub fn url(&self) -> &str {
        &self.url
    }

    #[must_use]
    pub fn token(&self) -> &str {
        &self.token
    }
}

impl fmt::Debug for RelayEndpoint {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        f.debug_struct("RelayEndpoint")
            .field("url", &self.url)
            .field("token", &"<redacted>")
            .finish()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn debug_output_never_holds_the_token() {
        let endpoint = RelayEndpoint::new("ws://127.0.0.1:1", "s3cret-token");
        for text in [
            format!("{endpoint:?}"),
            format!("{endpoint:#?}"),
            format!("{:?}", Some(&endpoint)),
        ] {
            assert!(!text.contains("s3cret-token"), "{text}");
            assert!(text.contains("ws://127.0.0.1:1"), "{text}");
        }
    }

    #[test]
    fn serializes_for_the_page_with_the_token() {
        let endpoint = RelayEndpoint::new("ws://127.0.0.1:1", "t");
        assert_eq!(
            serde_json::to_string(&endpoint).unwrap(),
            r#"{"url":"ws://127.0.0.1:1","token":"t"}"#,
            "wire shape"
        );
    }
}
