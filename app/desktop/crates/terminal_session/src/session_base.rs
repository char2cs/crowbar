//! The daemon paths of one chat's terminal sessions.

use crate::error::{Error, Result};

/// Where a chat's terminal sessions live: `/v0/chats/:chatId/terminals`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SessionBase(String);

impl SessionBase {
    /// The sessions of `chat`.
    ///
    /// # Errors
    /// [`Error::ChatId`] when `chat` is empty or holds a character that would change the path.
    pub fn chat(chat: &str) -> Result<Self> {
        let plain = |c: char| c.is_ascii_alphanumeric() || c == '-' || c == '_';
        if chat.is_empty() || !chat.chars().all(plain) {
            return Err(Error::ChatId(chat.to_owned()));
        }
        Ok(Self(format!("/v0/chats/{chat}/terminals")))
    }

    /// `POST` creates a session here.
    #[must_use]
    pub fn create_path(&self) -> &str {
        &self.0
    }

    /// `DELETE` kills the session.
    #[must_use]
    pub fn kill_path(
        &self,
        session: &str,
    ) -> String {
        format!("{}/{session}", self.0)
    }

    /// The PTY stream of the session.
    #[must_use]
    pub fn stream_path(
        &self,
        session: &str,
    ) -> String {
        format!("{}/{session}/ws", self.0)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const REFUSED: [&str; 4] = ["", "a/b", "a?b", "a b"];

    #[test]
    fn paths_follow_the_daemon_routes() {
        let base = SessionBase::chat("c-1").unwrap();
        assert_eq!(base.create_path(), "/v0/chats/c-1/terminals");
        assert_eq!(base.kill_path("s1"), "/v0/chats/c-1/terminals/s1");
        assert_eq!(base.stream_path("s1"), "/v0/chats/c-1/terminals/s1/ws");
    }

    #[test]
    fn a_chat_id_cannot_change_the_path() {
        for chat in REFUSED {
            assert!(SessionBase::chat(chat).is_err(), "{chat:?} must be refused");
        }
    }
}
