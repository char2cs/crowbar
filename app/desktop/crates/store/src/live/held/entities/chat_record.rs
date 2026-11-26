//! A chat as the projects scope carries it.

use daemonapi::Switch;
use serde::Deserialize;

use crate::live::held::entities::pending_move::PendingMove;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct ChatRecord {
    pub(crate) id: String,
    pub(crate) workspace_id: String,
    pub(crate) title: String,
    pub(crate) live_runner_id: String,
    pub(crate) active_provider_id: String,
    pub(crate) working: Switch,
    pub(crate) parent_id: String,
    pub(crate) order: i64,
    pub(crate) created_at: String,
    pub(crate) project_id: String,
    pub(crate) repo_id: String,
    pub(crate) pending_move: Option<PendingMove>,
}
