use crate::error::Result;
use crate::rest::Rest;
use crate::rest::session_paths::SessionPaths;

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Binding {
    Create,
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
    use super::*;
    use crate::test_support::{CREATED, LISTED, NONE_LISTED, daemon_answering, rest};

    #[tokio::test]
    async fn a_view_gets_a_new_session_or_its_saved_one_while_the_daemon_lists_it() {
        check_all().await;
    }

    async fn check_all() {
        let saved = || Binding::Saved("s1".to_owned());
        let cases = [
            (Binding::Create, NONE_LISTED, Some("s1"), "created"),
            (saved(), LISTED, Some("s1"), "saved and listed"),
            (saved(), NONE_LISTED, None, "saved but gone"),
        ];
        for (binding, listed, expected, name) in cases {
            let rest = rest(&daemon_answering(CREATED, listed));
            let base = SessionPaths::chat("c1").unwrap();
            let session = binding.session(&rest, &base).await.unwrap();
            assert_eq!(session.as_deref(), expected, "{name}");
        }
    }
}
