use crate::error::Error;
use crate::shell::shell_state::persistence::save_report::SaveReport;

#[derive(Debug, Default)]
pub(crate) struct SaveHealth {
    failing: bool,
}

impl SaveHealth {
    pub(crate) fn record(
        &mut self,
        outcome: Result<(), Error>,
    ) -> Option<SaveReport> {
        let was_failing = std::mem::replace(&mut self.failing, outcome.is_err());
        match (outcome, was_failing) {
            (Err(error), false) => Some(SaveReport::Failed(error.to_string())),
            (Ok(()), true) => Some(SaveReport::Recovered),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;

    use super::*;

    type Step = Result<(), &'static str>;

    struct Case {
        name: &'static str,
        steps: &'static [Step],
        expected: &'static [Option<&'static str>],
    }

    const CASES: &[Case] = &[
        Case {
            name: "healthy saves are silent",
            steps: &[Ok(()), Ok(())],
            expected: &[None, None],
        },
        Case {
            name: "a streak of failures is reported once",
            steps: &[Err("full"), Err("full"), Err("full")],
            expected: &[Some("failed"), None, None],
        },
        Case {
            name: "recovery is reported once and a new failure again",
            steps: &[Err("a"), Ok(()), Ok(()), Err("b")],
            expected: &[Some("failed"), Some("recovered"), None, Some("failed")],
        },
    ];

    fn failure_of(step: &Step) -> Result<(), Error> {
        step.map_err(|reason| Error::Write(PathBuf::from("layout"), std::io::Error::other(reason)))
    }

    fn label(report: &SaveReport) -> &'static str {
        match report {
            SaveReport::Failed(_) => "failed",
            SaveReport::Recovered => "recovered",
        }
    }

    #[test]
    fn failures_and_recoveries_are_reported_on_change_only() {
        for case in CASES {
            let mut health = SaveHealth::default();
            let seen: Vec<_> = case
                .steps
                .iter()
                .map(|step| health.record(failure_of(step)))
                .collect();
            let labels: Vec<_> = seen
                .iter()
                .map(|report| report.as_ref().map(label))
                .collect();
            assert_eq!(&labels, case.expected, "{}", case.name);
        }
    }
}
