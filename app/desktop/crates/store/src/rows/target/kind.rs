//! What a sidebar row stands for in the daemon's model.

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Kind {
    /// A grouping folder.
    Folder,
    /// A repository header, which is its default checkout's owning chat.
    RepoHeader {
        /// The repository's own checkout, where its threads run.
        workspace: String,
    },
    /// A branch row, which is its workspace's owning chat.
    Branch {
        /// The workspace.
        workspace: String,
        /// The chat that owns it; empty while the daemon has not named one.
        owner: String,
        /// Protected against rename and removal.
        locked: bool,
    },
    /// A chat.
    Chat {
        /// The workspace it runs in.
        workspace: String,
    },
}
