use crate::chat_locks::ChatLocks;
use crate::chat_servers::ChatServers;
use crate::chat_workspaces::ChatWorkspaces;

pub(crate) struct ChatReads {
    pub(crate) locks: ChatLocks,
    pub(crate) workspaces: ChatWorkspaces,
    pub(crate) servers: ChatServers,
}
