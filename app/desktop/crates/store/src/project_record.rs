//! A project (space) as the projects scope carries it.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct ProjectRecord {
    pub(crate) id: String,
    pub(crate) name: String,
    pub(crate) path: String,
    pub(crate) order: i64,
    pub(crate) avatar_emoji: String,
    pub(crate) avatar_url: String,
}
