//! Which workspace a delete removes, which rows are locked, and where the
//! selection lands afterwards.
//!
//! Ported from `web/src/lib/store/sidebar.ts`'s `collectDeletedIds`,
//! `isWorkspaceLockedInSidebar` and `getPostDeleteNavigationTarget`.

use std::collections::{HashMap, HashSet, VecDeque};

use crowbar_proto::domain::WorkspaceStatus;

use super::tree::{SidebarRepo, SidebarWorkspace};

/// Whether a status is the protected-branch lock.
///
/// A locked worktree refuses every daemon write with `409 workspace locked`,
/// so every mutation control gates on this.
#[must_use]
fn is_locked(status: Option<&WorkspaceStatus>) -> bool {
    matches!(status, Some(WorkspaceStatus::Locked))
}

/// Whether `ws_id` names a locked workspace.
///
/// Checks **both** id spaces a workspace can live in: the repo's tree rows,
/// and the default (main-worktree) workspace, which is never a tree row — it
/// exists only as [`SidebarRepo::default_workspace_id`] with its status lifted
/// onto [`SidebarRepo::default_status`]. Adopted protected branches are locked
/// *and* default, so missing that branch un-gated every repo home.
#[must_use]
pub fn is_workspace_locked(repos: &[SidebarRepo], ws_id: Option<&str>) -> bool {
    let Some(ws_id) = ws_id.filter(|id| !id.is_empty()) else {
        return false;
    };
    for repo in repos {
        if repo.default_workspace_id.as_deref() == Some(ws_id) {
            return is_locked(repo.default_status.as_ref());
        }
        if let Some(ws) = repo.workspaces.iter().find(|w| w.id == ws_id) {
            return is_locked(ws.status.as_ref());
        }
    }
    false
}

/// The workspace ids that deleting `ws_id` removes: the target plus all its
/// descendants, **skipping locked subtrees**.
///
/// A locked row stops the walk at itself — it is not deleted, and neither is
/// anything under it, because the daemon would refuse. If `ws_id` is itself
/// locked the result is empty, and the caller's delete is a no-op.
///
/// `all_workspaces` is every workspace across every repo, matching the TS
/// caller's `repos.flatMap(r => r.workspaces)`. It is indexed once so the walk
/// is linear rather than a scan per queued id.
#[must_use]
pub fn collect_deleted_ids(all_workspaces: &[SidebarWorkspace], ws_id: &str) -> HashSet<String> {
    let by_id: HashMap<&str, &SidebarWorkspace> = all_workspaces
        .iter()
        .map(|ws| (ws.id.as_str(), ws))
        .collect();

    let mut children_by_parent: HashMap<&str, Vec<&str>> = HashMap::new();
    for ws in all_workspaces {
        if let Some(parent) = ws.parent_id.as_deref().filter(|p| !p.is_empty()) {
            children_by_parent
                .entry(parent)
                .or_default()
                .push(ws.id.as_str());
        }
    }

    let mut to_delete: HashSet<String> = HashSet::new();
    let mut queue: VecDeque<&str> = VecDeque::from([ws_id]);
    while let Some(id) = queue.pop_front() {
        if to_delete.contains(id) {
            continue;
        }
        if by_id
            .get(id)
            .is_some_and(|ws| is_locked(ws.status.as_ref()))
        {
            continue;
        }
        to_delete.insert(id.to_owned());
        if let Some(children) = children_by_parent.get(id) {
            queue.extend(children.iter().copied());
        }
    }
    to_delete
}

/// Where the selection goes after deleting `ws_id`, when it — or one of its
/// descendants — is the active workspace.
///
/// Its parent if the parent survives, else the repo's base (locked) workspace,
/// else any surviving workspace in the repo, else `None` and the caller falls
/// back to the project. Mirrors `getPostDeleteNavigationTarget`.
///
/// Resolve this **before** the delete mutates the tree: afterwards the row is
/// gone and its parent is unknowable.
#[must_use]
pub fn post_delete_target(repos: &[SidebarRepo], ws_id: &str) -> Option<String> {
    let repo = repos
        .iter()
        .find(|r| r.workspaces.iter().any(|w| w.id == ws_id))?;
    let ws = repo.workspaces.iter().find(|w| w.id == ws_id)?;

    let all: Vec<SidebarWorkspace> = repos
        .iter()
        .flat_map(|r| r.workspaces.iter().cloned())
        .collect();
    let deleted = collect_deleted_ids(&all, ws_id);

    if let Some(parent) = ws.parent_id.as_deref().filter(|p| !p.is_empty())
        && !deleted.contains(parent)
    {
        return Some(parent.to_owned());
    }

    let mut survivors = repo.workspaces.iter().filter(|w| !deleted.contains(&w.id));
    let base = survivors
        .clone()
        .find(|w| is_locked(w.status.as_ref()))
        .or_else(|| survivors.next());
    base.map(|w| w.id.clone())
}

#[cfg(test)]
mod tests {
    use crowbar_proto::domain::WorkspaceStatus;

    use super::{collect_deleted_ids, is_workspace_locked, post_delete_target};
    use crate::sidebar::fixtures::{child_of, sidebar_repo, sidebar_workspace};
    use crate::sidebar::tree::{SidebarRepo, SidebarWorkspace};

    fn locked(id: &str) -> SidebarWorkspace {
        SidebarWorkspace {
            status: Some(WorkspaceStatus::Locked),
            ..sidebar_workspace(id)
        }
    }

    fn sorted(set: &std::collections::HashSet<String>) -> Vec<String> {
        let mut out: Vec<String> = set.iter().cloned().collect();
        out.sort();
        out
    }

    // --- lock gating -------------------------------------------------------

    #[test]
    fn a_tree_row_reports_its_own_lock() {
        let repos = vec![sidebar_repo(
            "r1",
            vec![locked("w1"), sidebar_workspace("w2")],
        )];
        assert!(is_workspace_locked(&repos, Some("w1")));
        assert!(!is_workspace_locked(&repos, Some("w2")));
    }

    /// The default workspace is never a tree row — it exists only as
    /// `default_workspace_id`, with its status lifted. Missing this branch
    /// un-gated every repo home.
    #[test]
    fn the_default_workspace_reports_its_lifted_lock() {
        let repo = SidebarRepo {
            default_workspace_id: Some("w-default".to_string()),
            default_status: Some(WorkspaceStatus::Locked),
            ..sidebar_repo("r1", vec![sidebar_workspace("w1")])
        };
        assert!(is_workspace_locked(&[repo], Some("w-default")));
    }

    #[test]
    fn an_unlocked_default_workspace_is_not_gated() {
        let repo = SidebarRepo {
            default_workspace_id: Some("w-default".to_string()),
            default_status: Some(WorkspaceStatus::New),
            ..sidebar_repo("r1", vec![])
        };
        assert!(!is_workspace_locked(&[repo], Some("w-default")));
    }

    #[test]
    fn no_workspace_is_not_locked() {
        let repos = vec![sidebar_repo("r1", vec![locked("w1")])];
        assert!(!is_workspace_locked(&repos, None));
        assert!(!is_workspace_locked(&repos, Some("")));
        assert!(!is_workspace_locked(&repos, Some("unknown")));
    }

    // --- the delete set ----------------------------------------------------

    #[test]
    fn deleting_a_leaf_removes_only_it() {
        let rows = vec![sidebar_workspace("a"), child_of("b", "a")];
        assert_eq!(sorted(&collect_deleted_ids(&rows, "b")), ["b"]);
    }

    #[test]
    fn deleting_a_parent_removes_its_descendants() {
        let rows = vec![
            sidebar_workspace("a"),
            child_of("b", "a"),
            child_of("c", "b"),
            sidebar_workspace("unrelated"),
        ];
        assert_eq!(sorted(&collect_deleted_ids(&rows, "a")), ["a", "b", "c"]);
    }

    /// The daemon refuses a locked worktree, so the walk stops at one — and
    /// everything under it survives too.
    #[test]
    fn a_locked_row_stops_the_walk_at_itself() {
        let rows = vec![
            sidebar_workspace("a"),
            locked("b"),
            child_of("c", "b"),
            child_of("d", "a"),
        ];
        let mut rows = rows;
        rows[1].parent_id = Some("a".to_string());
        assert_eq!(sorted(&collect_deleted_ids(&rows, "a")), ["a", "d"]);
    }

    #[test]
    fn deleting_a_locked_row_deletes_nothing() {
        let rows = vec![locked("a"), child_of("b", "a")];
        assert!(collect_deleted_ids(&rows, "a").is_empty());
    }

    /// A looped `parent_id` off the wire must not spin the walk.
    #[test]
    fn a_cycle_terminates() {
        let rows = vec![child_of("a", "b"), child_of("b", "a")];
        assert_eq!(sorted(&collect_deleted_ids(&rows, "a")), ["a", "b"]);
    }

    // --- where the selection lands ----------------------------------------

    #[test]
    fn the_surviving_parent_wins() {
        let repos = vec![sidebar_repo(
            "r1",
            vec![sidebar_workspace("a"), child_of("b", "a")],
        )];
        assert_eq!(post_delete_target(&repos, "b").as_deref(), Some("a"));
    }

    #[test]
    fn a_deleted_parent_falls_through_to_the_locked_base() {
        let repos = vec![sidebar_repo(
            "r1",
            vec![
                sidebar_workspace("a"),
                child_of("b", "a"),
                locked("base"),
                sidebar_workspace("other"),
            ],
        )];
        // Deleting `a` also deletes `b`, so `b`'s parent does not survive.
        assert_eq!(post_delete_target(&repos, "a").as_deref(), Some("base"));
    }

    #[test]
    fn with_no_locked_base_any_survivor_is_taken() {
        let repos = vec![sidebar_repo(
            "r1",
            vec![sidebar_workspace("a"), sidebar_workspace("survivor")],
        )];
        assert_eq!(post_delete_target(&repos, "a").as_deref(), Some("survivor"));
    }

    #[test]
    fn deleting_the_last_row_leaves_nowhere_to_go() {
        let repos = vec![sidebar_repo("r1", vec![sidebar_workspace("a")])];
        assert_eq!(post_delete_target(&repos, "a"), None);
    }

    #[test]
    fn an_unknown_row_has_no_target() {
        let repos = vec![sidebar_repo("r1", vec![sidebar_workspace("a")])];
        assert_eq!(post_delete_target(&repos, "ghost"), None);
    }

    /// The delete set spans every repo, matching the TS caller's flatMap, but
    /// the survivor search is scoped to the deleted row's own repo.
    #[test]
    fn the_target_stays_inside_the_deleted_rows_repo() {
        let repos = vec![
            sidebar_repo("r1", vec![sidebar_workspace("a")]),
            sidebar_repo("r2", vec![sidebar_workspace("elsewhere")]),
        ];
        assert_eq!(post_delete_target(&repos, "a"), None);
    }
}
