#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Kind {
    Folder,
    RepoHeader {
        workspace: String,
    },
    Branch {
        workspace: String,
        owner: String,
        locked: bool,
    },
    Chat {
        workspace: String,
    },
}
