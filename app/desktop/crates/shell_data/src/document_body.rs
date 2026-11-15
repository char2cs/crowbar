//! A document as `GET /v0/documents/content` answers.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct DocumentBody {
    pub(crate) content: String,
    pub(crate) encoding: String,
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_content_is_read_and_the_encoding_may_be_absent() {
        let body: DocumentBody = serde_json::from_str(r#"{"content":"hi"}"#).unwrap();
        assert_eq!((body.content.as_str(), body.encoding.as_str()), ("hi", ""));
    }
}
