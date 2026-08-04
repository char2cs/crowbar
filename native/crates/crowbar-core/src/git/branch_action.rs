//! Resolve the branch section's primary + secondary action from the current
//! repo state — a pure, precedence-ordered decision function.
//!
//! Ported from `web/src/features/git/lib/branch-action.ts`.
//!
//! Precedence: uncommitted (commit first) > conflicts-with-parent >
//! protected > mergeable > sync-only. A branch that conflicts with its
//! parent — whether an active merge conflict or a predicted one — arrives
//! here as `status == "pr-conflicts"` (the daemon folds the prediction into
//! the status), so the single [`BranchActionKind::Resolve`] case covers both.
//! The remote secondary action is only offered on a clean tree (you commit
//! before you push/pull); behind wins over ahead when diverged.
//!
//! `status` is kept a plain `&str`, matching the TS source's own
//! `BranchActionInput.status: string` — deliberately not narrowed to
//! `crowbar_proto::domain::WorkspaceStatus`, even though the one production
//! caller (`branch-section.tsx`) is ultimately fed a `WorkspaceDTO.status`
//! value. The TS decision function only ever compares one literal
//! (`"pr-conflicts"`); reusing the wire enum here would couple a
//! domain-generic decision function to one particular DTO for no behavioural
//! gain, which the TS source's own interface avoids too.
//!
//! # Mutation-tested: the resolve/pull-request precedence
//!
//! Precedence bugs are the classic way a chain of `else if`s goes wrong
//! silently — swapped branches still compile and still return *a* variant, so
//! only a test that pins ordering (not just each case in isolation) can
//! catch it. Temporarily swapped the `Resolve`/`PullRequest` branches' order
//! and ran `cargo test -p crowbar-core --lib git::branch_action`. Real
//! output:
//!
//! ```text
//! test git::branch_action::tests::conflicts_with_parent_resolve_ahead_of_merge_pull_request ... FAILED
//! thread '...' panicked at crates/crowbar-core/src/git/branch_action.rs:132:9:
//! assertion `left == right` failed
//!   left: PullRequest
//!  right: Resolve
//! test result: FAILED. 6 passed; 1 failed; 0 ignored; 0 measured; 117 filtered out
//! ```
//!
//! The other six tests still passed — only the precedence-ordering case
//! caught it, confirming it's the one doing the work. Reverted immediately
//! after.

/// Which primary action the branch section offers, given the repo state.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BranchActionKind {
    /// Uncommitted changes -> open the commit dialog.
    Commit,
    /// Conflicts with the parent must be resolved (rebase onto parent).
    Resolve,
    /// Parent is protected -> open a PR.
    PullRequest,
    /// Mergeable into parent -> open the merge popover.
    Merge,
    /// No parent branch -> push/pull only.
    SyncOnly,
}

/// The remote secondary action shown alongside the primary — only offered
/// when the tree is clean.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RemoteAction {
    Push,
    Pull,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct BranchActionInput<'a> {
    pub has_uncommitted: bool,
    pub has_parent: bool,
    pub can_merge_locally: bool,
    pub status: &'a str,
    pub ahead: i64,
    pub behind: i64,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct BranchAction {
    pub kind: BranchActionKind,
    pub remote: Option<RemoteAction>,
}

/// Mirrors `resolveBranchAction`.
#[must_use]
pub fn resolve_branch_action(input: &BranchActionInput<'_>) -> BranchAction {
    let remote = if input.has_uncommitted {
        None
    } else if input.behind > 0 {
        Some(RemoteAction::Pull)
    } else if input.ahead > 0 {
        Some(RemoteAction::Push)
    } else {
        None
    };

    let kind = if input.has_uncommitted {
        BranchActionKind::Commit
    } else if input.has_parent && input.status == "pr-conflicts" {
        BranchActionKind::Resolve
    } else if input.has_parent && !input.can_merge_locally {
        BranchActionKind::PullRequest
    } else if input.has_parent && input.can_merge_locally {
        BranchActionKind::Merge
    } else {
        BranchActionKind::SyncOnly
    };

    BranchAction { kind, remote }
}

#[cfg(test)]
mod tests {
    use super::{BranchActionInput, BranchActionKind, RemoteAction, resolve_branch_action};

    const BASE: BranchActionInput<'static> = BranchActionInput {
        has_uncommitted: false,
        has_parent: true,
        can_merge_locally: true,
        status: "new",
        ahead: 0,
        behind: 0,
    };

    // --- ported from web/src/__tests__/features/git/lib/branch-action.test.ts ---

    #[test]
    fn uncommitted_changes_commit_overrides_everything_no_remote_secondary() {
        let action = resolve_branch_action(&BranchActionInput {
            has_uncommitted: true,
            ahead: 2,
            status: "pr-conflicts",
            ..BASE
        });
        assert_eq!(action.kind, BranchActionKind::Commit);
        assert_eq!(action.remote, None);
    }

    #[test]
    fn conflicts_with_parent_resolve_ahead_of_merge_pull_request() {
        // The daemon folds a predicted conflict into pr-conflicts, so the
        // single resolve case covers both an active and a predicted
        // conflict. It wins even when the parent looks mergeable or
        // protected.
        assert_eq!(
            resolve_branch_action(&BranchActionInput {
                status: "pr-conflicts",
                ..BASE
            })
            .kind,
            BranchActionKind::Resolve
        );
        assert_eq!(
            resolve_branch_action(&BranchActionInput {
                status: "pr-conflicts",
                can_merge_locally: false,
                ..BASE
            })
            .kind,
            BranchActionKind::Resolve
        );
    }

    #[test]
    fn clean_and_protected_parent_pull_request() {
        assert_eq!(
            resolve_branch_action(&BranchActionInput {
                can_merge_locally: false,
                ..BASE
            })
            .kind,
            BranchActionKind::PullRequest
        );
    }

    #[test]
    fn clean_and_mergeable_parent_merge() {
        assert_eq!(resolve_branch_action(&BASE).kind, BranchActionKind::Merge);
    }

    #[test]
    fn clean_and_no_parent_sync_only() {
        assert_eq!(
            resolve_branch_action(&BranchActionInput {
                has_parent: false,
                ..BASE
            })
            .kind,
            BranchActionKind::SyncOnly
        );
    }

    #[test]
    fn remote_secondary_ahead_pushes_behind_pulls_diverged_pulls_synced_none() {
        assert_eq!(
            resolve_branch_action(&BranchActionInput { ahead: 1, ..BASE }).remote,
            Some(RemoteAction::Push)
        );
        assert_eq!(
            resolve_branch_action(&BranchActionInput { behind: 1, ..BASE }).remote,
            Some(RemoteAction::Pull)
        );
        assert_eq!(
            resolve_branch_action(&BranchActionInput {
                ahead: 1,
                behind: 1,
                ..BASE
            })
            .remote,
            Some(RemoteAction::Pull)
        );
        assert_eq!(resolve_branch_action(&BASE).remote, None);
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn an_unrecognised_status_with_a_parent_and_local_merge_still_merges() {
        // "pr-conflicts" is the only status this function special-cases;
        // every other value (including ones the daemon might add later) must
        // fall through to the mergeable/protected branches unchanged.
        assert_eq!(
            resolve_branch_action(&BranchActionInput {
                status: "pr-open",
                ..BASE
            })
            .kind,
            BranchActionKind::Merge
        );
    }
}
