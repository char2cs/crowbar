//! The delete of a held row, and what its refusal tells the shell.

mod risk_record;

use sidebar::{RowId, WorkAtRisk};

use crate::error::Error;
use crate::live::Outcome;
use crate::live::PlanError;

const WORK_AT_RISK: &str = "work_at_risk";

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum Removal {
    KeepWork(RowId),
    DiscardWork(RowId),
}

impl Removal {
    pub(crate) fn row(&self) -> &RowId {
        match self {
            Self::KeepWork(row) | Self::DiscardWork(row) => row,
        }
    }

    pub(crate) fn discards_work(&self) -> bool {
        matches!(self, Self::DiscardWork(_))
    }

    pub(crate) fn refused(
        &self,
        error: &PlanError,
    ) -> Outcome {
        let row = self.row().clone();
        match (self, at_risk_of(error)) {
            (Self::KeepWork(_), Some(at_risk)) => Outcome::WorkAtRisk { row, at_risk },
            _ => Outcome::RemovalRefused {
                row,
                reason: error.to_string(),
            },
        }
    }
}

fn at_risk_of(error: &PlanError) -> Option<Vec<WorkAtRisk>> {
    let PlanError::Daemon(Error::Daemon(daemonapi::Error::Refused {
        code: Some(code),
        data,
        ..
    })) = error
    else {
        return None;
    };
    (code == WORK_AT_RISK).then(|| risk_record::listed(data))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn refusal(
        code: Option<&str>,
        data: serde_json::Value,
    ) -> PlanError {
        PlanError::Daemon(Error::Daemon(daemonapi::Error::Refused {
            path: "/v0/x".into(),
            status: 409,
            code: code.map(str::to_owned),
            message: "delete would destroy work".into(),
            data: Box::new(data),
        }))
    }

    fn at_risk() -> serde_json::Value {
        serde_json::json!({"workAtRisk": [{"workspaceId": "w", "branch": "feat", "uncommittedFiles": 2, "unmergedCommits": 1}]})
    }

    fn feat() -> WorkAtRisk {
        WorkAtRisk {
            branch: "feat".into(),
            uncommitted_files: 2,
            unmerged_commits: 1,
        }
    }

    fn refused_with(reason: &str) -> Outcome {
        Outcome::RemovalRefused {
            row: RowId::from("b"),
            reason: reason.into(),
        }
    }

    #[test]
    fn only_a_delete_sent_without_consent_asks_again_over_work_at_risk() {
        let keep = Removal::KeepWork(RowId::from("b"));
        let discard = Removal::DiscardWork(RowId::from("b"));
        let risky = refusal(Some(WORK_AT_RISK), at_risk());
        let other = refusal(Some("conflict"), serde_json::Value::Null);
        let asked = Outcome::WorkAtRisk {
            row: RowId::from("b"),
            at_risk: vec![feat()],
        };
        let cases = [
            ("asks", keep.refused(&risky), asked),
            (
                "consented",
                discard.refused(&risky),
                refused_with("delete would destroy work"),
            ),
            (
                "other",
                keep.refused(&other),
                refused_with("delete would destroy work"),
            ),
            (
                "loading",
                keep.refused(&PlanError::ChatLoading),
                refused_with("its chat is still loading — try again in a moment"),
            ),
        ];
        for (name, outcome, expected) in cases {
            assert_eq!(outcome, expected, "{name}");
        }
    }
}
