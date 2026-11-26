//! A new thread that has no chat yet: develop's draft pane, created on its first send.

/// Where a draft thread's chat will be created once its first prompt is sent.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DraftThread {
    /// The draft's id, `draft-` and a fresh uuid, which no daemon chat has.
    pub id: String,
    /// The space the chat will be created in.
    pub project: String,
    /// The repository of its workspace; empty for the space's home.
    pub repo: String,
    /// The workspace it will run in.
    pub workspace: String,
    /// The row it will be placed under; empty for the panel root.
    pub parent: String,
}

const PREFIX: &str = "draft-";

impl DraftThread {
    /// Whether `chat` names a draft rather than a chat of the daemon.
    #[must_use]
    pub fn is_draft(chat: &str) -> bool {
        chat.starts_with(PREFIX)
    }

    pub(crate) fn minted(uuid: &str) -> String {
        format!("{PREFIX}{uuid}")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_minted_id_is_a_draft_and_a_daemon_chat_is_not() {
        assert!(DraftThread::is_draft(&DraftThread::minted("u1")));
        assert!(!DraftThread::is_draft("u1"));
    }
}
