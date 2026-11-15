//! What a workspace record means for a branch row.

use sidebar::{Workspace, WorkspaceStatus};

use crate::workspace_record::WorkspaceRecord;

const LOCKED: &str = "locked";
const PLACEHOLDER: &str = "placeholder";

pub(crate) fn status(record: &WorkspaceRecord) -> WorkspaceStatus {
    match record.status.as_str() {
        LOCKED => WorkspaceStatus::Locked,
        _ if record.provisioning == PLACEHOLDER => WorkspaceStatus::Placeholder,
        "pr-conflicts" => WorkspaceStatus::PrConflicts,
        "deleted" => WorkspaceStatus::Deleted,
        "pr-closed" => WorkspaceStatus::PrClosed,
        "pr-open" => WorkspaceStatus::PrOpen,
        "pr-merged" => WorkspaceStatus::PrMerged,
        _ => WorkspaceStatus::New,
    }
}

pub(crate) fn is_locked(record: &WorkspaceRecord) -> bool {
    record.status == LOCKED
}

pub(crate) fn landed(record: &WorkspaceRecord) -> Workspace {
    Workspace {
        status: status(record),
        branch: record.branch.clone(),
        added: record.added,
        deleted: record.deleted,
        held_by_path: Some(record.held_by_path.clone()).filter(|path| !path.is_empty()),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn record(
        status: &str,
        provisioning: &str,
    ) -> WorkspaceRecord {
        WorkspaceRecord {
            status: status.into(),
            provisioning: provisioning.into(),
            ..WorkspaceRecord::default()
        }
    }

    #[test]
    fn status_follows_the_daemon_and_placeholders_need_provisioning() {
        let cases = [
            (("new", "provisioned"), WorkspaceStatus::New),
            (("pr-open", "provisioned"), WorkspaceStatus::PrOpen),
            (("pr-merged", ""), WorkspaceStatus::PrMerged),
            (("locked", "placeholder"), WorkspaceStatus::Locked),
            (("new", "placeholder"), WorkspaceStatus::Placeholder),
            (("deleted", ""), WorkspaceStatus::Deleted),
        ];
        for ((status, provisioning), expected) in cases {
            assert_eq!(
                super::status(&record(status, provisioning)),
                expected,
                "{status}"
            );
        }
    }

    #[test]
    fn landed_reports_a_holder_only_when_there_is_one() {
        let mut held = record("locked", "placeholder");
        held.held_by_path = "/repo".into();
        assert_eq!(landed(&held).held_by_path.as_deref(), Some("/repo"));
        assert_eq!(landed(&record("new", "")).held_by_path, None);
        assert!(is_locked(&held));
    }
}
