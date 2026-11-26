//! A document as `GET /v0/documents/content` answers.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(default)]
pub(crate) struct DocumentBody {
    pub(crate) content: String,
    pub(crate) encoding: String,
}
