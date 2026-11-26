use crate::panes::editor::chat_locks::ChatLocks;
use crate::panes::editor::chat_servers::ChatServers;
use crate::panes::editor::chat_workspaces::ChatWorkspaces;

pub(crate) struct ChatReads {
    pub(crate) locks: ChatLocks,
    pub(crate) workspaces: ChatWorkspaces,
    pub(crate) servers: ChatServers,
}
