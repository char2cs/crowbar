//! Dragging a workspace row: what a drop lands on, and whether it is legal.
//!
//! Ported from `web/src/components/layout/workspace-tree-context.tsx` and the
//! `reparentWorkspace` reducer in `web/src/lib/store/sidebar.ts`.
//!
//! # A repo header is a highlight, not a drop
//!
//! `findDropTarget` resolves three kinds of target — the trash, a workspace
//! row, and a repo header — but the drop handler acts on only two of them.
//! There is no `repo:` branch in `onPointerUp`; the id is produced solely so
//! `RepoSection` can light up while the pointer is over it. That asymmetry is
//! reproduced here as [`DropOutcome::Nothing`] rather than quietly given a
//! meaning the reference does not have.

use super::tree::{SidebarRepo, SidebarWorkspace};

/// What the pointer is currently over.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DropTarget {
    /// The trash strip at the foot of the tree.
    Trash,
    /// A workspace row, by id. Never the dragged row itself — the hit test
    /// skips it, which is what stops a self-drop flickering the highlight.
    Workspace(String),
    /// A repo header, by id. Highlight only — see the module docs.
    Repo(String),
}

/// What releasing the pointer over a target should do.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum DropOutcome {
    /// Delete the dragged workspace.
    Delete(String),
    /// Move the dragged workspace under a new parent.
    Reparent {
        /// The workspace being moved.
        ws_id: String,
        /// Its new parent.
        new_parent_id: String,
    },
    /// The drop is a no-op: an illegal move, a cross-repo target, or a repo
    /// header.
    Nothing,
}

/// Whether `ws_id` may be reparented under `new_parent_id` within `repo`.
///
/// Three conditions, all from the `reparentWorkspace` reducer:
///
/// 1. the workspace exists in this repo;
/// 2. the new parent exists **in the same repo** — `None` means "make it a
///    root", which is always allowed for a row that exists;
/// 3. the move closes no cycle. The walk climbs from the proposed parent and
///    rejects on reaching `ws_id` (the move itself would loop) or on revisiting
///    any ancestor (the existing chain is already looped, and without this
///    guard the walk would not terminate).
#[must_use]
pub fn can_reparent(repo: &SidebarRepo, ws_id: &str, new_parent_id: Option<&str>) -> bool {
    if !repo.workspaces.iter().any(|w| w.id == ws_id) {
        return false;
    }
    let Some(new_parent_id) = new_parent_id else {
        return true;
    };
    if !repo.workspaces.iter().any(|w| w.id == new_parent_id) {
        return false;
    }
    !closes_cycle(&repo.workspaces, ws_id, new_parent_id)
}

/// Whether climbing from `from` reaches `ws_id` or loops.
fn closes_cycle(workspaces: &[SidebarWorkspace], ws_id: &str, from: &str) -> bool {
    let mut visited: Vec<&str> = Vec::new();
    let mut cursor = Some(from);
    while let Some(at) = cursor {
        if at == ws_id || visited.contains(&at) {
            return true;
        }
        visited.push(at);
        cursor = workspaces
            .iter()
            .find(|w| w.id == at)
            .and_then(|w| w.parent_id.as_deref())
            .filter(|p| !p.is_empty());
    }
    false
}

/// Resolve a released drag into the action it should take.
///
/// `dragged_repo_id` is the repo the dragged row belongs to. A workspace drop
/// whose target lives in a **different** repo is rejected: the reference finds
/// the target's repo and compares it to the dragged row's before doing
/// anything, so dragging across repo sections is inert rather than a move.
#[must_use]
pub fn resolve_drop(
    repos: &[SidebarRepo],
    dragged_ws_id: &str,
    dragged_repo_id: &str,
    target: Option<&DropTarget>,
) -> DropOutcome {
    match target {
        Some(DropTarget::Trash) => DropOutcome::Delete(dragged_ws_id.to_owned()),
        Some(DropTarget::Workspace(target_ws_id)) => {
            if target_ws_id == dragged_ws_id {
                return DropOutcome::Nothing;
            }
            let target_repo = repos
                .iter()
                .find(|r| r.workspaces.iter().any(|w| w.id == *target_ws_id));
            let Some(target_repo) = target_repo.filter(|r| r.id == dragged_repo_id) else {
                return DropOutcome::Nothing;
            };
            if can_reparent(target_repo, dragged_ws_id, Some(target_ws_id)) {
                DropOutcome::Reparent {
                    ws_id: dragged_ws_id.to_owned(),
                    new_parent_id: target_ws_id.clone(),
                }
            } else {
                DropOutcome::Nothing
            }
        }
        Some(DropTarget::Repo(_)) | None => DropOutcome::Nothing,
    }
}

#[cfg(test)]
mod tests {
    use super::{DropOutcome, DropTarget, can_reparent, resolve_drop};
    use crate::sidebar::fixtures::{child_of, sidebar_repo, sidebar_workspace};

    fn repo_with_chain() -> super::SidebarRepo {
        // a -> b -> c
        sidebar_repo(
            "r1",
            vec![
                sidebar_workspace("a"),
                child_of("b", "a"),
                child_of("c", "b"),
            ],
        )
    }

    // --- legality ----------------------------------------------------------

    #[test]
    fn a_row_may_move_under_an_unrelated_sibling() {
        let repo = sidebar_repo("r1", vec![sidebar_workspace("a"), sidebar_workspace("b")]);
        assert!(can_reparent(&repo, "a", Some("b")));
    }

    #[test]
    fn a_row_may_be_promoted_to_a_root() {
        assert!(can_reparent(&repo_with_chain(), "c", None));
    }

    #[test]
    fn an_unknown_row_may_not_be_moved() {
        assert!(!can_reparent(&repo_with_chain(), "ghost", None));
        assert!(!can_reparent(&repo_with_chain(), "ghost", Some("a")));
    }

    #[test]
    fn a_parent_outside_the_repo_is_rejected() {
        assert!(!can_reparent(&repo_with_chain(), "a", Some("elsewhere")));
    }

    #[test]
    fn a_row_may_not_become_its_own_parent() {
        assert!(!can_reparent(&repo_with_chain(), "a", Some("a")));
    }

    /// Moving `a` under its own descendant would loop the tree.
    #[test]
    fn a_move_under_a_descendant_is_rejected() {
        let repo = repo_with_chain();
        assert!(!can_reparent(&repo, "a", Some("b")));
        assert!(!can_reparent(&repo, "a", Some("c")));
        assert!(!can_reparent(&repo, "b", Some("c")));
    }

    /// The second guard. Without the visited set this walk never terminates.
    #[test]
    fn an_already_looped_chain_is_rejected_rather_than_walked_forever() {
        let repo = sidebar_repo(
            "r1",
            vec![
                child_of("x", "y"),
                child_of("y", "x"),
                sidebar_workspace("free"),
            ],
        );
        assert!(!can_reparent(&repo, "free", Some("x")));
    }

    // --- resolving a drop --------------------------------------------------

    #[test]
    fn a_trash_drop_deletes_the_dragged_row() {
        let repos = vec![repo_with_chain()];
        assert_eq!(
            resolve_drop(&repos, "c", "r1", Some(&DropTarget::Trash)),
            DropOutcome::Delete("c".to_string())
        );
    }

    #[test]
    fn a_legal_workspace_drop_reparents() {
        let repos = vec![repo_with_chain()];
        assert_eq!(
            resolve_drop(
                &repos,
                "c",
                "r1",
                Some(&DropTarget::Workspace("a".to_string()))
            ),
            DropOutcome::Reparent {
                ws_id: "c".to_string(),
                new_parent_id: "a".to_string(),
            }
        );
    }

    #[test]
    fn an_illegal_workspace_drop_does_nothing() {
        let repos = vec![repo_with_chain()];
        assert_eq!(
            resolve_drop(
                &repos,
                "a",
                "r1",
                Some(&DropTarget::Workspace("c".to_string()))
            ),
            DropOutcome::Nothing
        );
    }

    #[test]
    fn dropping_a_row_on_itself_does_nothing() {
        let repos = vec![repo_with_chain()];
        assert_eq!(
            resolve_drop(
                &repos,
                "a",
                "r1",
                Some(&DropTarget::Workspace("a".to_string()))
            ),
            DropOutcome::Nothing
        );
    }

    /// The reference finds the target's repo and compares it to the dragged
    /// row's before doing anything, so a cross-section drag is inert.
    #[test]
    fn a_cross_repo_drop_does_nothing() {
        let repos = vec![
            repo_with_chain(),
            sidebar_repo("r2", vec![sidebar_workspace("other")]),
        ];
        assert_eq!(
            resolve_drop(
                &repos,
                "a",
                "r1",
                Some(&DropTarget::Workspace("other".to_string()))
            ),
            DropOutcome::Nothing
        );
    }

    #[test]
    fn a_drop_on_an_unknown_row_does_nothing() {
        let repos = vec![repo_with_chain()];
        assert_eq!(
            resolve_drop(
                &repos,
                "a",
                "r1",
                Some(&DropTarget::Workspace("ghost".to_string()))
            ),
            DropOutcome::Nothing
        );
    }

    /// `findDropTarget` produces a repo id, and `onPointerUp` has no branch
    /// for it. The highlight is the whole feature.
    #[test]
    fn a_repo_header_is_a_highlight_not_a_drop() {
        let repos = vec![repo_with_chain()];
        assert_eq!(
            resolve_drop(&repos, "c", "r1", Some(&DropTarget::Repo("r1".to_string()))),
            DropOutcome::Nothing
        );
    }

    #[test]
    fn releasing_over_nothing_does_nothing() {
        let repos = vec![repo_with_chain()];
        assert_eq!(resolve_drop(&repos, "c", "r1", None), DropOutcome::Nothing);
    }
}
