//! The daemon paths of one chat's terminal sessions.

use crate::error::{Error, Result};

/// Where a chat's terminal sessions live: `/v0/chats/:chatId/terminals`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SessionPaths(String);

impl SessionPaths {
    /// The sessions of `chat`.
    ///
    /// # Errors
    /// [`Error::ChatId`] when `chat` is empty or holds a character that would change the path.
    pub fn chat(chat: &str) -> Result<Self> {
        checked(chat)?;
        Ok(Self(format!("/v0/chats/{chat}/terminals")))
    }

    /// The sessions of the home of `project`, which has no chat.
    ///
    /// # Errors
    /// [`Error::ChatId`] when `project` is empty or holds a character that would change the path.
    pub fn project_home(project: &str) -> Result<Self> {
        checked(project)?;
        Ok(Self(format!("/v0/projects/{project}/home/terminals")))
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

fn checked(id: &str) -> Result<()> {
    let plain = |c: char| c.is_ascii_alphanumeric() || c == '-' || c == '_';
    if id.is_empty() || !id.chars().all(plain) {
        return Err(Error::ChatId(id.to_owned()));
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, &str, &str); 2] = [
        (
            "c-1",
            "/v0/chats/c-1/terminals",
            "/v0/chats/c-1/terminals/s1/ws",
        ),
        (
            "p1",
            "/v0/projects/p1/home/terminals",
            "/v0/projects/p1/home/terminals/s1/ws",
        ),
    ];
    const REFUSED: [&str; 4] = ["", "a/b", "a?b", "a b"];

    #[test]
    fn paths_follow_the_daemon_routes() {
        let bases = [
            SessionPaths::chat(CASES[0].0).unwrap(),
            SessionPaths::project_home(CASES[1].0).unwrap(),
        ];
        for (base, case) in bases.iter().zip(CASES) {
            assert_eq!(base.create_path(), case.1);
            assert_eq!(base.stream_path("s1"), case.2);
        }
        assert_eq!(bases[0].kill_path("s1"), "/v0/chats/c-1/terminals/s1");
    }

    #[test]
    fn an_id_cannot_change_the_path() {
        for id in REFUSED {
            assert!(SessionPaths::chat(id).is_err(), "{id:?} must be refused");
            assert!(
                SessionPaths::project_home(id).is_err(),
                "{id:?} must be refused"
            );
        }
    }
}
