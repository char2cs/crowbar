//! Placeholder-workspace detection and its human-readable reason.
//!
//! Ported from `web/src/lib/workspace/placeholder.ts`. A placeholder workspace
//! is one that has no on-disk worktree: its branch could not be materialised,
//! most often because a live worktree already has it checked out (spec §3.3).

use crowbar_proto::api_v0_dto::WorkspaceDTO;

/// A placeholder workspace is one with no on-disk worktree. The missing
/// `local_path` **is** the signal — status is deliberately not part of it.
/// Protected-branch placeholders are seeded `locked` to inherit the
/// protection guards, but an imported feature branch must stay unlocked
/// (`locked` survives provisioning and would block merge/rename/delete
/// forever), so requiring `locked` here left every failed-import row
/// unrecognised: no reason, no Retry/Detach…, no toast. Mirrors
/// `isPlaceholderWorkspace`.
///
/// `Some("")` counts the same as `None` — see `crate::workspace` module docs.
#[must_use]
pub fn is_placeholder_workspace(ws: &WorkspaceDTO) -> bool {
    ws.local_path.as_deref().unwrap_or("").is_empty()
}

/// Reconstruct the human-readable reason a placeholder exists. A live holder
/// is the actionable case and wins, because it names the checkout the user has
/// to detach. Otherwise fall back to the cause the daemon recorded on the row
/// — a failure with no holder has no reconstructable reason, and the generic
/// line would hide the only explanation there is (spec §3.3/§4/B7). Mirrors
/// `placeholderReason`.
#[must_use]
pub fn placeholder_reason(ws: &WorkspaceDTO) -> String {
    if let Some(held_by_path) = non_empty(ws.held_by_path.as_deref()) {
        return format!(
            "`{}` is checked out at {held_by_path} — detach it to let Crowbar manage this branch.",
            ws.branch
        );
    }
    if let Some(last_error) = non_empty(ws.last_error.as_deref()) {
        return format!("Crowbar couldn't set up `{}`: {last_error}", ws.branch);
    }
    format!(
        "Crowbar couldn't set up `{}`. Retry to provision it.",
        ws.branch
    )
}

/// `if (x)` on a JS-optional string treats `""` the same as absent. See
/// `crate::workspace` module docs for why this equivalence is kept rather than
/// narrowed to `Option::is_some`.
fn non_empty(s: Option<&str>) -> Option<&str> {
    s.filter(|s| !s.is_empty())
}

#[cfg(test)]
mod tests {
    use super::{is_placeholder_workspace, placeholder_reason};
    use crowbar_proto::api_v0_dto::WorkspaceDTO;
    use crowbar_proto::domain_git::MergeStrategy;

    /// Minimal, valid `WorkspaceDTO`, with every field this module reads left
    /// at its "unset" value — mirrors the TS test file's `ws()` factory.
    fn workspace() -> WorkspaceDTO {
        WorkspaceDTO {
            id: "w1".to_string(),
            repo_id: "r1".to_string(),
            project_id: "p1".to_string(),
            kind: None,
            branch: "develop".to_string(),
            parent_id: None,
            fork_point_sha: None,
            status: None,
            working: false,
            last_error: None,
            is_default: None,
            added: 0,
            deleted: 0,
            merge_strategy: MergeStrategy::Other(String::new()),
            can_merge_locally: false,
            merge_conflicts: false,
            parent_branch: None,
            pr_url: None,
            pr_title: None,
            pr_target_branch: None,
            local_path: None,
            held_by_path: None,
        }
    }

    // --- ported from web/src/__tests__/lib/workspace/placeholder.test.ts ---

    #[test]
    fn is_true_for_a_locked_workspace_with_no_local_path() {
        let ws = WorkspaceDTO {
            held_by_path: Some("/repo".to_string()),
            ..workspace()
        };
        assert!(is_placeholder_workspace(&ws));
    }

    #[test]
    fn is_false_for_a_locked_workspace_that_has_a_local_path() {
        let ws = WorkspaceDTO {
            local_path: Some("/managed".to_string()),
            ..workspace()
        };
        assert!(!is_placeholder_workspace(&ws));
    }

    #[test]
    fn is_false_for_a_healthy_non_locked_workspace() {
        let ws = WorkspaceDTO {
            local_path: Some("/managed".to_string()),
            ..workspace()
        };
        assert!(!is_placeholder_workspace(&ws));
    }

    // An imported feature branch whose worktree could not be created is a
    // placeholder too, and it is deliberately NOT locked (locked survives
    // provisioning and would block merge/rename/delete forever). Requiring
    // locked here is what left the import's failed row unrecognised — no
    // reason, no Retry/Detach…, and no toast.
    #[test]
    fn is_true_for_an_unlocked_import_placeholder_held_by_another_worktree() {
        let ws = WorkspaceDTO {
            held_by_path: Some("/Users/me/other".to_string()),
            ..workspace()
        };
        assert!(is_placeholder_workspace(&ws));
    }

    #[test]
    fn names_the_branch_and_the_holder_path_when_known() {
        let ws = WorkspaceDTO {
            held_by_path: Some("/Users/me/repo".to_string()),
            ..workspace()
        };
        let reason = placeholder_reason(&ws);
        assert!(reason.contains("develop"));
        assert!(reason.contains("/Users/me/repo"));
    }

    #[test]
    fn falls_back_to_a_generic_reason_without_a_holder_path() {
        let reason = placeholder_reason(&workspace());
        assert!(reason.contains("develop"));
    }

    // A failure with no live holder has no reconstructable reason, so the
    // daemon persists the cause on the row; showing "Retry to provision it"
    // instead would hide the only explanation the user can act on.
    #[test]
    fn prefers_the_recorded_cause_when_there_is_no_holder_path() {
        let ws = WorkspaceDTO {
            last_error: Some("worktree add: disk full".to_string()),
            ..workspace()
        };
        assert!(placeholder_reason(&ws).contains("worktree add: disk full"));
    }

    #[test]
    fn still_prefers_the_holder_path_over_a_recorded_cause() {
        let ws = WorkspaceDTO {
            held_by_path: Some("/Users/me/other".to_string()),
            last_error: Some("branch_already_exists".to_string()),
            ..workspace()
        };
        assert!(placeholder_reason(&ws).contains("/Users/me/other"));
    }

    // --- new: the empty-string/absent equivalence, not exercised by the TS suite ---

    #[test]
    fn an_empty_local_path_counts_as_a_placeholder_too() {
        let ws = WorkspaceDTO {
            local_path: Some(String::new()),
            ..workspace()
        };
        assert!(is_placeholder_workspace(&ws));
    }

    #[test]
    fn an_empty_held_by_path_falls_through_to_the_recorded_cause() {
        let ws = WorkspaceDTO {
            held_by_path: Some(String::new()),
            last_error: Some("worktree add: disk full".to_string()),
            ..workspace()
        };
        assert!(placeholder_reason(&ws).contains("worktree add: disk full"));
    }

    #[test]
    fn an_empty_last_error_falls_through_to_the_generic_reason() {
        let ws = WorkspaceDTO {
            last_error: Some(String::new()),
            ..workspace()
        };
        let reason = placeholder_reason(&ws);
        assert!(reason.contains("Retry to provision it"));
    }
}
