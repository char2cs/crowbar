use crate::plan::Plan;

impl Plan {
    #[must_use]
    pub(crate) fn failing_as(
        mut self,
        title: Option<String>,
    ) -> Self {
        self.failure = title;
        self
    }

    #[must_use]
    pub(crate) fn explaining(
        mut self,
        reason: &str,
    ) -> Self {
        self.explained = Some(reason.to_owned());
        self
    }

    #[must_use]
    pub(crate) fn alerting(
        mut self,
        lead: String,
    ) -> Self {
        self.alerting = Some(lead);
        self
    }
}

#[cfg(test)]
mod tests {
    use crate::plan::Plan;

    #[test]
    fn a_failure_is_titled_explained_and_alerted_as_asked() {
        let plan = Plan::of("local", Vec::new())
            .failing_as(Some("Move Failed".into()))
            .explaining("Try again.")
            .alerting("Failed to move a.rs".into());
        assert_eq!(plan.failure.as_deref(), Some("Move Failed"));
        assert_eq!(plan.explained.as_deref(), Some("Try again."));
        assert_eq!(plan.alerting.as_deref(), Some("Failed to move a.rs"));
    }
}
