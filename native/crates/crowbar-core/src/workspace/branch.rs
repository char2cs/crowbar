//! Branch -> owning-workspace-id lookup within a repo.
//!
//! Ported from `web/src/lib/workspace/branch-workspace.ts`.

use crowbar_proto::api_v0_dto::WorkspaceDTO;

/// Returns the id of the Crowbar-managed workspace that already holds
/// `branch` among `workspaces`, or `None` if the branch is free. Exact,
/// case-sensitive match (git branch semantics). Mirrors
/// `findWorkspaceForBranch`.
///
/// `workspaces` is expected to already exclude the default (main-worktree)
/// workspace — the TS source's own comment: that workspace is the imported
/// repo folder, which merely sits on some branch and must never block
/// creating/importing that same branch as a real managed workspace. The TS
/// caller filters it out of `repo.workspaces` before this function ever sees
/// the list; this port keeps the same contract rather than re-deciding what
/// "default" means here, since that classification lives on `RepoDTO`, not on
/// any single `WorkspaceDTO`.
#[must_use]
pub fn find_workspace_for_branch(workspaces: &[WorkspaceDTO], branch: &str) -> Option<String> {
    let name = branch.trim();
    if name.is_empty() {
        return None;
    }
    workspaces
        .iter()
        .find(|w| w.branch == name)
        .map(|w| w.id.clone())
}

#[cfg(test)]
mod tests {
    use super::find_workspace_for_branch;
    use crowbar_proto::api_v0_dto::WorkspaceDTO;
    use crowbar_proto::domain_git::MergeStrategy;

    fn workspace(id: &str, branch: &str) -> WorkspaceDTO {
        WorkspaceDTO {
            id: id.to_string(),
            repo_id: "r1".to_string(),
            project_id: "p1".to_string(),
            kind: None,
            branch: branch.to_string(),
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

    fn repo_workspaces() -> Vec<WorkspaceDTO> {
        // The default/main-worktree workspace ("develop") is deliberately NOT
        // in this list, matching the TS fixture's `repo.workspaces` (the
        // default workspace is already filtered out upstream).
        vec![workspace("c1", "feature/x"), workspace("c2", "spike/y")]
    }

    // --- ported from web/src/__tests__/lib/workspace/branch-workspace.test.ts ---

    #[test]
    fn matches_an_existing_managed_child_branch() {
        assert_eq!(
            find_workspace_for_branch(&repo_workspaces(), "feature/x"),
            Some("c1".to_string())
        );
    }

    #[test]
    fn does_not_match_the_default_branch_the_repo_folder_is_unmanaged() {
        assert_eq!(
            find_workspace_for_branch(&repo_workspaces(), "develop"),
            None
        );
    }

    #[test]
    fn returns_none_for_a_free_branch() {
        assert_eq!(
            find_workspace_for_branch(&repo_workspaces(), "feature/new"),
            None
        );
    }

    #[test]
    fn is_case_sensitive() {
        assert_eq!(
            find_workspace_for_branch(&repo_workspaces(), "Feature/X"),
            None
        );
    }

    #[test]
    fn trims_surrounding_whitespace_before_matching() {
        assert_eq!(
            find_workspace_for_branch(&repo_workspaces(), "  feature/x  "),
            Some("c1".to_string())
        );
    }

    #[test]
    fn returns_none_for_an_empty_or_whitespace_branch() {
        assert_eq!(find_workspace_for_branch(&repo_workspaces(), "   "), None);
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn returns_none_for_an_empty_workspace_list() {
        assert_eq!(find_workspace_for_branch(&[], "feature/x"), None);
    }
}
