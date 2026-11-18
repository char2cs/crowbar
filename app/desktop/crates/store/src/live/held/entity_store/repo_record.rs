//! A repository as the projects scope carries it.

use serde::Deserialize;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct RepoRecord {
    pub(crate) id: String,
    pub(crate) project_id: String,
    pub(crate) name: String,
    pub(crate) default_branch: String,
    pub(crate) avatar_label: String,
    pub(crate) avatar_color: String,
    pub(crate) avatar_emoji: String,
    pub(crate) order: i64,
    pub(crate) deleting: bool,
}
