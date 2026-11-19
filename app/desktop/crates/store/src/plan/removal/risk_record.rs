//! One workspace a delete was refused over, as the daemon's refusal names it.

use serde::Deserialize;
use serde_json::Value;
use sidebar::WorkAtRisk;

const LIST_FIELD: &str = "workAtRisk";

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct RiskRecord {
    branch: String,
    uncommitted_files: u32,
    unmerged_commits: u32,
}

impl RiskRecord {
    fn into_work(self) -> WorkAtRisk {
        WorkAtRisk {
            branch: self.branch,
            uncommitted_files: self.uncommitted_files,
            unmerged_commits: self.unmerged_commits,
        }
    }
}

pub(crate) fn listed(data: &Value) -> Vec<WorkAtRisk> {
    let Some(list) = data.get(LIST_FIELD) else {
        return Vec::new();
    };
    match Vec::<RiskRecord>::deserialize(list) {
        Ok(records) => records.into_iter().map(RiskRecord::into_work).collect(),
        Err(error) => {
            tracing::warn!(%error, "the work at risk of a refused delete does not parse");
            Vec::new()
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_refusal_names_each_branch_and_what_it_would_lose() {
        let cases = [
            (
                serde_json::json!({"workAtRisk": [{"workspaceId": "w", "branch": "feat", "uncommittedFiles": 0, "unmergedCommits": 1}]}),
                vec![WorkAtRisk {
                    branch: "feat".into(),
                    uncommitted_files: 0,
                    unmerged_commits: 1,
                }],
            ),
            (serde_json::json!({}), Vec::new()),
            (serde_json::json!({"workAtRisk": "nope"}), Vec::new()),
        ];
        for (data, expected) in cases {
            assert_eq!(listed(&data), expected, "{data}");
        }
    }
}
