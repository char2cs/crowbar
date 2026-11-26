use crate::error::{Error, Result};

#[derive(Debug, Default)]
pub(crate) struct Failures {
    first: Option<Error>,
}

impl Failures {
    pub(crate) fn note(
        &mut self,
        result: Result<()>,
    ) {
        let Err(error) = result else {
            return;
        };
        tracing::warn!(%error, "a pane could not be driven; the others are still reconciled");
        self.first.get_or_insert(error);
    }

    pub(crate) fn into_first(self) -> Option<Error> {
        self.first
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn keeps_the_first_error_and_ignores_successes() {
        let mut failures = Failures::default();
        failures.note(Ok(()));
        assert!(failures.first.is_none(), "no failure yet");
        failures.note(Err(Error::NotShown(protocol::PaneId::new("a").unwrap())));
        failures.note(Err(Error::NotShown(protocol::PaneId::new("b").unwrap())));
        let first = failures.into_first().map(|error| error.to_string());
        assert_eq!(first.as_deref(), Some("pane a is not shown"));
    }
}
