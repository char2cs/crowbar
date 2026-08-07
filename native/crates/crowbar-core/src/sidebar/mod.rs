//! The sidebar's domain model — spec §4.2's workspace-scoping bucket, widened
//! by Slice 1a to cover everything the sidebar decides without a window.
//!
//! Ported from the React sidebar's logic half:
//!
//! | this module | ported from |
//! |---|---|
//! | [`tree`] | `web/src/lib/store/build-repo-tree.ts` |
//! | [`hierarchy`] | `web/src/components/layout/workspace-tree-utils.ts` |
//! | [`panel`] | `web/src/components/layout/use-sidebar-panel.ts` |
//! | [`tabs`] | `web/src/lib/store/sidebar.ts`, `sidebar-carousel.tsx` |
//! | [`collapse`] | `web/src/lib/store/sidebar.ts` |
//! | [`drag`] | `web/src/components/layout/workspace-tree-context.tsx` |
//! | [`selection`] | `web/src/lib/store/sidebar.ts` (delete set, lock, post-delete) |
//!
//! [`crate::workspace`] already owns scope resolution, placeholder
//! classification, branch lookup and keep-alive policy; this module consumes
//! those rather than restating them.
//!
//! # Why this is `core` and not `state`
//!
//! Every decision here is a pure function of daemon data plus user intent:
//! which rows nest under which, how wide the panel may be, whether a drop is
//! legal. None of it needs a window, so §7.1 puts it here where the ≥98%
//! coverage gate applies. `crowbar-state` wraps the results in `Entity<T>` and
//! owns nothing else.

pub mod collapse;
pub mod drag;
pub mod hierarchy;
pub mod panel;
pub mod selection;
pub mod tabs;
pub mod tree;

#[cfg(test)]
pub(crate) mod fixtures {
    //! Minimal valid records, so a test states only the fields it is about.
    //!
    //! Mirrors the TS suites' `ws()` / `repo()` factories: every field not
    //! named by a test sits at its unset value, so a test that cares about
    //! `parent_id` is not also silently asserting a status.

    use crowbar_proto::api_v0_dto::{RepoDTO, WorkspaceDTO};
    use crowbar_proto::domain_git::MergeStrategy;

    use super::tree::{AvatarSource, SidebarRepo, SidebarWorkspace};

    /// A workspace DTO with every optional field unset.
    pub fn workspace_dto(id: &str, repo_id: &str) -> WorkspaceDTO {
        WorkspaceDTO {
            id: id.to_string(),
            repo_id: repo_id.to_string(),
            project_id: "p1".to_string(),
            kind: None,
            branch: format!("branch-{id}"),
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
            local_path: Some(format!("/w/{id}")),
            held_by_path: None,
        }
    }

    /// A repo DTO carrying the daemon's own avatar shape.
    pub fn repo_dto(id: &str, project_id: &str) -> RepoDTO {
        RepoDTO {
            id: id.to_string(),
            project_id: project_id.to_string(),
            name: format!("repo-{id}"),
            path: format!("/r/{id}"),
            default_branch: "main".to_string(),
            avatar_label: "R".to_string(),
            avatar_color: "avatar-slate".to_string(),
            avatar_url: None,
            avatar_emoji: None,
            status: None,
        }
    }

    /// A sidebar row with every optional field unset.
    pub fn sidebar_workspace(id: &str) -> SidebarWorkspace {
        SidebarWorkspace {
            id: id.to_string(),
            branch: format!("branch-{id}"),
            parent_id: None,
            status: None,
            added: 0,
            deleted: 0,
            working: false,
            can_merge_locally: false,
            merge_conflicts: false,
            parent_branch: None,
            pr_url: None,
            last_error: String::new(),
            held_by_path: String::new(),
            local_path: Some(format!("/w/{id}")),
        }
    }

    /// A repo section holding `workspaces`.
    pub fn sidebar_repo(id: &str, workspaces: Vec<SidebarWorkspace>) -> SidebarRepo {
        SidebarRepo {
            id: id.to_string(),
            project_id: "p1".to_string(),
            name: format!("repo-{id}"),
            avatar_label: "R".to_string(),
            avatar_color: "avatar-slate".to_string(),
            avatar_source: AvatarSource::Initials,
            workspaces,
            default_workspace_id: None,
            default_branch: None,
            default_working: false,
            default_status: None,
            local_path: Some(format!("/r/{id}")),
        }
    }

    /// A row with a parent.
    pub fn child_of(id: &str, parent: &str) -> SidebarWorkspace {
        SidebarWorkspace {
            parent_id: Some(parent.to_string()),
            ..sidebar_workspace(id)
        }
    }

    /// Compared within a margin because `clippy::pedantic` denies `float_cmp`
    /// and `clippy.toml`'s test exemptions cover only `unwrap`/`expect`/
    /// `panic`. Every value asserted through this is an exact small integer —
    /// a pixel width or a scroll offset — so the margin is a membership test,
    /// not a tolerance for accumulated error. Reaching for `#[allow]` here
    /// would be indistinguishable from silencing the lint in shipping code,
    /// which is the thing §4.3 rule 4 exists to prevent.
    pub trait ApproxPx {
        /// Whether two pixel values name the same pixel.
        fn approx_eq(&self, other: &Self) -> bool;
    }

    impl ApproxPx for f32 {
        fn approx_eq(&self, other: &Self) -> bool {
            (self - other).abs() < 1e-3
        }
    }

    impl ApproxPx for Option<f32> {
        fn approx_eq(&self, other: &Self) -> bool {
            match (self, other) {
                (Some(a), Some(b)) => a.approx_eq(b),
                (None, None) => true,
                _ => false,
            }
        }
    }

    /// `assert_eq!` for pixel values. Same two arities.
    macro_rules! assert_px {
        ($actual:expr, $expected:expr $(,)?) => {{
            let (actual, expected) = ($actual, $expected);
            assert!(
                $crate::sidebar::fixtures::ApproxPx::approx_eq(&actual, &expected),
                "expected {expected:?} px, got {actual:?} px"
            );
        }};
        ($actual:expr, $expected:expr, $($msg:tt)+) => {{
            let (actual, expected) = ($actual, $expected);
            assert!(
                $crate::sidebar::fixtures::ApproxPx::approx_eq(&actual, &expected),
                "expected {expected:?} px, got {actual:?} px: {}",
                format_args!($($msg)+)
            );
        }};
    }

    pub(crate) use assert_px;
}
