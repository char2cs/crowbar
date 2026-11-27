use std::io;

use crate::error::Error;

#[derive(Debug, thiserror::Error)]
pub(crate) enum PlanError {
    #[error("this space is not loaded yet")]
    SpaceNotLoaded,
    #[error("this space has no home to run a thread in yet")]
    NoHome,
    #[error("this row is not loaded any more")]
    RowNotLoaded,
    #[error("this chat is not loaded any more")]
    ChatNotLoaded,
    #[error("this workspace is not loaded yet")]
    WorkspaceNotLoaded,
    #[error("no space is open")]
    NoSpaceOpen,
    #[error("no workspace is open")]
    NoWorkspaceOpen,
    #[error("no workspace is open to change files in")]
    NoWorkspaceToChange,
    #[error("a project home has no worktree to fork")]
    HomeHasNoWorktree,
    #[error("this folder has no name")]
    FolderHasNoName,
    #[error("this row cannot go there")]
    CannotGoThere,
    #[error("there is nothing to dismiss")]
    NothingToDismiss,
    #[error("start a thread from a workspace row, a folder has none to run it in")]
    NoWorkspaceToRunIn,
    #[error("its chat is still loading — try again in a moment")]
    ChatLoading,
    #[error("a folder has no chat")]
    FolderHasNoChat,
    #[error("The icon must be a png, jpeg or webp image.")]
    IconType,
    #[error("The icon could not be read: {0}")]
    IconUnreadable(#[source] io::Error),
    #[error("{0} is not a data command")]
    NotADataCommand(String),
    #[error("host {0} is not known")]
    UnknownHost(String),
    #[error("no agent is enabled on this host; enable one in Settings")]
    NoAgentEnabled,
    #[error("That branch hasn't been checked out yet — try again once it has")]
    ForkNotCheckedOut,
    #[error("{0}")]
    Operation(String),
    #[error("{}", .0.message)]
    HostUnanswered(daemonapi::HostNotice),
    #[error("{}", .0.reason())]
    Daemon(#[source] Error),
}

impl From<PlanError> for sidebar::Error {
    fn from(error: PlanError) -> Self {
        Self::Refused {
            reason: error.to_string(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn each_refusal_reads_as_the_sentence_the_panel_shows() {
        let refused = Error::Daemon(daemonapi::Error::Refused {
            path: "/v0/x".into(),
            status: 409,
            code: None,
            message: "name taken".into(),
            data: Box::default(),
        });
        let cases = [
            (PlanError::SpaceNotLoaded, "this space is not loaded yet"),
            (PlanError::Daemon(refused), "name taken"),
            (PlanError::UnknownHost("h".into()), "host h is not known"),
            (
                PlanError::NotADataCommand("Quit".into()),
                "Quit is not a data command",
            ),
        ];
        for (error, text) in cases {
            assert_eq!(error.to_string(), text);
        }
    }
}
