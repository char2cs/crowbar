//! Whether a session is made for the view or already exists.

use crate::error::Result;
use crate::rest::Rest;
use crate::session_base::SessionBase;

#[derive(Debug, Clone)]
pub(crate) enum Binding {
    Create,
    Existing(String),
}

impl Binding {
    pub(crate) async fn session(
        &self,
        rest: &Rest,
        base: &SessionBase,
    ) -> Result<String> {
        match self {
            Self::Create => rest.create(base).await,
            Self::Existing(session) => Ok(session.clone()),
        }
    }

    pub(crate) async fn release(
        &self,
        rest: &Rest,
        base: &SessionBase,
        session: &str,
    ) {
        if !matches!(self, Self::Create) {
            return;
        }
        if let Err(error) = rest.kill(base, session).await {
            tracing::debug!(%error, "the terminal session was already gone");
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_a_created_session_is_the_views_to_kill() {
        let cases = [
            (Binding::Create, true),
            (Binding::Existing("agent".to_owned()), false),
        ];
        for (binding, owned) in cases {
            assert_eq!(matches!(binding, Binding::Create), owned, "{binding:?}");
        }
    }
}
