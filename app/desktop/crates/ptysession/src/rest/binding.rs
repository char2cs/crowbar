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
        }
    }
}

#[cfg(test)]
mod tests {
    use protocol::HostId;

    use super::*;
    use crate::test_support::{CREATED, LISTED, NONE_LISTED, daemon_answering};

    #[tokio::test]
    async fn a_view_gets_a_new_session_or_its_saved_one_while_the_daemon_lists_it() {
        let saved = || Binding::Saved("s1".to_owned());
        let created = session_of(Binding::Create, NONE_LISTED).await;
        assert_eq!(created.as_deref(), Some("s1"), "created");
        let kept = session_of(saved(), LISTED).await;
        assert_eq!(kept.as_deref(), Some("s1"), "saved and listed");
        assert_eq!(
            session_of(saved(), NONE_LISTED).await,
            None,
            "saved but gone"
        );
    }

    async fn session_of(
        binding: Binding,
        listed: &'static str,
    ) -> Option<String> {
        let rest = Rest {
            transport: daemon_answering(CREATED, listed),
            host: HostId::new("local").unwrap(),
        };
        let base = SessionPaths::chat("c1").unwrap();
        binding.session(&rest, &base).await.unwrap()
    }
}
