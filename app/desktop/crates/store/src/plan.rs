//! What one sidebar command asks of the daemon.

use daemonapi::Call;

use crate::after::After;
use crate::pending_row::PendingRow;
use crate::surface::Surface;

#[derive(Debug, Clone, PartialEq)]
pub(crate) struct Plan {
    pub(crate) host: String,
    pub(crate) calls: Vec<Call>,
    pub(crate) pending: Option<PendingRow>,
    pub(crate) after: After,
    pub(crate) dropping: Option<String>,
    pub(crate) needs_provider: bool,
    pub(crate) landing: Surface,
    pub(crate) committing: bool,
}

impl Plan {
    pub(crate) fn of(
        host: &str,
        calls: Vec<Call>,
    ) -> Self {
        Self {
            host: host.to_owned(),
            calls,
            pending: None,
            after: After::Nothing,
            dropping: None,
            needs_provider: false,
            landing: Surface::Default,
            committing: false,
        }
    }

    #[must_use]
    pub(crate) fn showing(
        mut self,
        row: PendingRow,
    ) -> Self {
        self.pending = Some(row);
        self
    }

    #[must_use]
    pub(crate) fn then(
        mut self,
        after: After,
    ) -> Self {
        self.after = after;
        self
    }

    #[must_use]
    pub(crate) fn committing(mut self) -> Self {
        self.committing = true;
        self
    }

    #[must_use]
    pub(crate) fn landing_on(
        mut self,
        landing: Surface,
    ) -> Self {
        self.landing = landing;
        self
    }

    #[must_use]
    pub(crate) fn with_provider(mut self) -> Self {
        self.needs_provider = true;
        self
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use daemonapi::Call;

    use crate::pending_row::PendingRow;

    #[test]
    fn builders_add_the_row_the_follow_up_and_the_provider_need() {
        let row = PendingRow {
            id: "n".into(),
            label: "New".into(),
            scope: String::new(),
            parent: String::new(),
            failure: None,
            operation: None,
        };
        let plan = Plan::of("local", vec![Call::get("/v0/x")])
            .showing(row)
            .then(After::OpenChat("n".into()))
            .with_provider();
        assert_eq!(plan.host, "local");
        assert_eq!(plan.after, After::OpenChat("n".into()));
        assert!(plan.needs_provider);
        assert_eq!(plan.pending.map(|row| row.id), Some("n".to_owned()));
        assert!(plan.dropping.is_none());
    }
}
