#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DraftThread {
    pub id: String,
    pub project: String,
    pub repo: String,
    pub workspace: String,
    pub parent: String,
}

const PREFIX: &str = "draft-";

impl DraftThread {
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
