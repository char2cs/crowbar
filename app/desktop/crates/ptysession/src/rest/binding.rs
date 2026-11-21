//! Whether a session is made for the view or already exists.

use crate::error::Result;
use crate::rest::Rest;
use crate::rest::session_paths::SessionPaths;

/// How a terminal view is bound to a daemon session.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Binding {
    /// A new session is made for the view, reported as [`Report::Bound`](crate::Report::Bound)
    /// and killed when the view goes.
    Create,
    /// The session the view was bound to before, attached if the daemon still lists it, else
    /// reported as [`Report::Gone`](crate::Report::Gone); killed when the view goes.
    Saved(String),
    /// A session someone else owns, such as an agent runner's PTY: never created or killed.
    Existing(String),
}

impl Binding {
    pub(crate) async fn session(
        &self,
        rest: &Rest,
        base: &SessionPaths,
    ) -> Result<Option<String>> {
        match self {
            Self::Create => rest.create(base).await.map(Some),
            Self::Saved(session) => {
                let live = rest.live(base).await?;
                Ok(live.contains(session).then(|| session.clone()))
            },
            Self::Existing(session) => Ok(Some(session.clone())),
        }
    }

    pub(crate) async fn release(
        &self,
        rest: &Rest,
        base: &SessionPaths,
        session: &str,
    ) -> Result<()> {
        if matches!(self, Self::Existing(_)) {
            return Ok(());
        }
        rest.kill(base, session).await
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_an_agents_session_is_never_the_views_to_kill() {
        let cases = [
            (Binding::Create, true),
            (Binding::Saved("s1".to_owned()), true),
            (Binding::Existing("agent".to_owned()), false),
        ];
        for (binding, owned) in cases {
            assert_eq!(
                !matches!(binding, Binding::Existing(_)),
                owned,
                "{binding:?}"
            );
        }
    }
}
