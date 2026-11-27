use crate::shell::chats::facets::workspace_status::WorkspaceStatus;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Workspace {
    pub status: WorkspaceStatus,
    pub branch: String,
    pub added: u32,
    pub deleted: u32,
    pub held_by_path: Option<String>,
}
