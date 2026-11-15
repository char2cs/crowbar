//! A workspace (a checkout of a branch) as the projects scope carries it.

use serde::Deserialize;

use crate::switch::Switch;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct WorkspaceRecord {
    pub(crate) id: String,
    pub(crate) repo_id: String,
    pub(crate) project_id: String,
    pub(crate) branch: String,
    pub(crate) parent_id: String,
    pub(crate) status: String,
    pub(crate) merge_strategy: String,
    pub(crate) can_merge_locally: bool,
    pub(crate) working: Switch,
    pub(crate) is_default: bool,
    pub(crate) added: u32,
    pub(crate) deleted: u32,
    pub(crate) provisioning: String,
    pub(crate) held_by_path: String,
    pub(crate) local_path: String,
    pub(crate) created_at: String,
    pub(crate) owning_chat_id: String,
    pub(crate) folder_id: String,
    pub(crate) order: i64,
}
