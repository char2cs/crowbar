//! A sidebar folder as the projects scope carries it.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct FolderRecord {
    pub(crate) id: String,
    pub(crate) project_id: String,
    pub(crate) repo_id: String,
    pub(crate) parent_id: String,
    pub(crate) name: String,
    pub(crate) order: i64,
    pub(crate) created_at: String,
}
