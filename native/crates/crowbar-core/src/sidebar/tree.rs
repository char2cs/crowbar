//! Flat daemon DTOs grouped into the nested shape the sidebar renders.
//!
//! Ported from `web/src/lib/store/build-repo-tree.ts`.
//!
//! # The default workspace is not a tree row, and three fields exist because of it
//!
//! Every repo has a *default* workspace — the imported repo folder itself,
//! sitting on whatever branch is checked out there. It is drawn as the **repo
//! header**, not as a row under it, so [`build_repo_tree`] filters it out of
//! [`SidebarRepo::workspaces`]. Filtering it out would also drop its live
//! state, so three of its fields are lifted onto the repo:
//! [`SidebarRepo::default_working`] (the header's spinner during an agent
//! turn), [`SidebarRepo::default_status`] (an adopted protected branch is
//! `locked` **and** default, and mutation gating has to see the lock), and
//! [`SidebarRepo::default_branch`] (create-input validation reserves it).
//!
//! # `avatar_color` is data this crate does not resolve
//!
//! It arrives as a string on the record and is carried verbatim. The daemon
//! writes one of eight palette tokens — `avatar-rose`, `avatar-amber`,
//! `avatar-emerald`, `avatar-cyan`, `avatar-indigo`, `avatar-violet`,
//! `avatar-slate`, `avatar-pink` (`api/internal/app/usecases/internal/avatar`)
//! — and **no `.avatar-*` rule exists anywhere in the reference stylesheets**,
//! so the reference app paints no background at all for them. That was
//! verified live, on the running React app, and is recorded in
//! `native/mapping/repo-avatar.md`: the node carries `class="… avatar-slate"`
//! and computes `background-color: rgba(0, 0, 0, 0)`.
//!
//! Resolving it is `crowbar-ui`'s job because a colour cannot be named here —
//! the design tokens are sealed newtypes and this crate has no access to them.
//! Carrying the string keeps the decision in one place instead of splitting it
//! across a layer boundary.
//!
//! # What is deliberately not ported
//!
//! * **`age`** — `toSidebarWorkspace` sets it to `''` unconditionally and no
//!   consumer in `web/src` reads `workspace.age`. Porting it would carry a
//!   field that is always empty and never displayed.
//! * **`repoAvatarLabel` / `repoAvatarColor`** — React's `||` fallbacks for an
//!   empty `avatarLabel`/`avatarColor`. The daemon sets both on every write
//!   path (`project.go:143`, `project_import.go:397`, `repos.go:415`), so the
//!   fallbacks never run against a live daemon. Their palette is also a
//!   *different* eight colours (`bg-indigo-700`, …) from the daemon's, so
//!   porting them would introduce a second palette that nothing can produce.
//!   An empty value is carried through as empty and rendered as no background,
//!   which is what the reference does with the values it actually sends.

use crowbar_proto::api_v0_dto::{RepoDTO, WorkspaceDTO};
use crowbar_proto::domain::WorkspaceStatus;

/// One workspace row in the sidebar tree.
///
/// Mirrors `toSidebarWorkspace`'s output. Fields the React object spreads
/// conditionally are [`Option`] here; the two it spreads **unconditionally**
/// on purpose — `last_error` and `held_by_path` — are plain [`String`], and
/// that difference is load-bearing. `applyWorkspaceDTO` merges frames with
/// `{...w, ...ws}`, so an error cleared by a successful Retry must *overwrite*
/// the stale value rather than be omitted and leave it lingering.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SidebarWorkspace {
    /// Workspace id, the tree's row identity.
    pub id: String,
    /// The git branch this workspace holds.
    pub branch: String,
    /// Parent workspace id, when this row nests under another.
    pub parent_id: Option<String>,
    /// The §5 seven-value lifecycle badge. `None` when the daemon sent none.
    pub status: Option<WorkspaceStatus>,
    /// Lines added against the parent branch.
    pub added: i64,
    /// Lines deleted against the parent branch.
    pub deleted: i64,
    /// True while an agent or long-running operation is in flight. A separate
    /// flag from [`Self::status`], not an overlay on it.
    pub working: bool,
    /// Whether this workspace can merge into its parent.
    pub can_merge_locally: bool,
    /// Whether merging into the parent is predicted to conflict.
    pub merge_conflicts: bool,
    /// Parent branch name, when mergeable.
    pub parent_branch: Option<String>,
    /// Open PR url, when the workspace has one.
    pub pr_url: Option<String>,
    /// Last background-operation error, surfaced to the user. Empty, never
    /// absent — see the type's own docs.
    pub last_error: String,
    /// Worktree directory holding this branch when the workspace is a
    /// placeholder. Empty, never absent — see the type's own docs.
    pub held_by_path: String,
    /// On-disk worktree directory. Absent on a placeholder, which is exactly
    /// what [`crate::workspace::placeholder::is_placeholder_workspace`] tests.
    pub local_path: Option<String>,
}

/// One repo section in the sidebar tree, with its workspace rows.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct SidebarRepo {
    /// Repo id.
    pub id: String,
    /// Owning project — used to derive the active project from a workspace.
    pub project_id: String,
    /// Display name.
    pub name: String,
    /// Single-character badge the daemon generated from the name.
    pub avatar_label: String,
    /// Avatar background, verbatim and unresolved. See the module docs.
    pub avatar_color: String,
    /// Where the repo's picture comes from.
    pub avatar_source: AvatarSource,
    /// The tree rows, default workspace excluded. See the module docs.
    pub workspaces: Vec<SidebarWorkspace>,
    /// Id of the default (main-worktree) workspace — the repo header opens it.
    pub default_workspace_id: Option<String>,
    /// Branch of the default workspace, reserved by create-input validation.
    pub default_branch: Option<String>,
    /// `working` of the default workspace, lifted. See the module docs.
    pub default_working: bool,
    /// `status` of the default workspace, lifted. See the module docs.
    pub default_status: Option<WorkspaceStatus>,
    /// On-disk root of the repo, the `local_path` fallback for the default
    /// workspace.
    pub local_path: Option<String>,
}

/// Which of the three pictures a repo's avatar draws.
///
/// React crams all three into one optional string, marking the emoji case with
/// an `emoji:` prefix it later slices back off. That encoding exists because
/// the prop is a single `url?: string`; it carries no information a three-way
/// choice does not, and `crowbar_ui`'s `repo_avatar::Kind` already models the
/// same three branches. Naming them here means the prefix is parsed once, at
/// the boundary, instead of at every call site.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum AvatarSource {
    /// A custom emoji, which takes precedence over a fetched icon.
    Emoji(String),
    /// The daemon's icon proxy path, e.g.
    /// `/v0/projects/<projectId>/repos/<id>/icon`. Relative: the caller joins
    /// it to the daemon origin, because only `crowbar-client` knows what that
    /// is.
    Icon(String),
    /// Neither — the letter fallback.
    Initials,
}

impl AvatarSource {
    /// Resolve a repo's avatar source. Mirrors `repoAvatarURL`: a custom emoji
    /// wins, then the icon path, then the letter fallback.
    ///
    /// An empty string counts as absent, matching the `if (x)` the TS uses on
    /// both fields — see [`crate::workspace`] module docs for why that
    /// equivalence is kept rather than narrowed to [`Option::is_some`].
    #[must_use]
    pub fn of(repo: &RepoDTO) -> Self {
        if let Some(emoji) = non_empty(repo.avatar_emoji.as_deref()) {
            return Self::Emoji(emoji.to_owned());
        }
        if let Some(url) = non_empty(repo.avatar_url.as_deref()) {
            return Self::Icon(url.to_owned());
        }
        Self::Initials
    }
}

/// `if (x)` on a JS-optional string treats `""` the same as absent.
fn non_empty(s: Option<&str>) -> Option<&str> {
    s.filter(|s| !s.is_empty())
}

/// Convert one workspace DTO into its sidebar row. Mirrors
/// `toSidebarWorkspace`.
#[must_use]
pub fn to_sidebar_workspace(ws: &WorkspaceDTO) -> SidebarWorkspace {
    SidebarWorkspace {
        id: ws.id.clone(),
        branch: ws.branch.clone(),
        parent_id: non_empty(ws.parent_id.as_deref()).map(ToOwned::to_owned),
        status: ws.status.clone(),
        added: ws.added,
        deleted: ws.deleted,
        working: ws.working,
        can_merge_locally: ws.can_merge_locally,
        merge_conflicts: ws.merge_conflicts,
        parent_branch: non_empty(ws.parent_branch.as_deref()).map(ToOwned::to_owned),
        pr_url: non_empty(ws.pr_url.as_deref()).map(ToOwned::to_owned),
        last_error: ws.last_error.clone().unwrap_or_default(),
        held_by_path: ws.held_by_path.clone().unwrap_or_default(),
        local_path: non_empty(ws.local_path.as_deref()).map(ToOwned::to_owned),
    }
}

/// Group one repo's workspaces under it. Mirrors `toSidebarRepo`.
///
/// `workspaces` may span every repo; only rows whose `repo_id` matches are
/// taken.
#[must_use]
pub fn to_sidebar_repo(repo: &RepoDTO, workspaces: &[WorkspaceDTO]) -> SidebarRepo {
    let owned = workspaces.iter().filter(|ws| ws.repo_id == repo.id);
    let default_ws = owned.clone().find(|ws| ws.is_default.unwrap_or(false));

    SidebarRepo {
        id: repo.id.clone(),
        project_id: repo.project_id.clone(),
        name: repo.name.clone(),
        avatar_label: repo.avatar_label.clone(),
        avatar_color: repo.avatar_color.clone(),
        avatar_source: AvatarSource::of(repo),
        workspaces: owned
            .filter(|ws| !ws.is_default.unwrap_or(false))
            .map(to_sidebar_workspace)
            .collect(),
        default_workspace_id: default_ws.map(|ws| ws.id.clone()),
        default_branch: default_ws.map(|ws| ws.branch.clone()),
        default_working: default_ws.is_some_and(|ws| ws.working),
        default_status: default_ws.and_then(|ws| ws.status.clone()),
        local_path: non_empty(Some(repo.path.as_str())).map(ToOwned::to_owned),
    }
}

/// Group a flat workspace list under its repos. Mirrors `buildRepoTree`.
#[must_use]
pub fn build_repo_tree(repos: &[RepoDTO], workspaces: &[WorkspaceDTO]) -> Vec<SidebarRepo> {
    repos
        .iter()
        .map(|repo| to_sidebar_repo(repo, workspaces))
        .collect()
}

/// [`build_repo_tree`] filtered to one project. Mirrors `buildScopedRepoTree`.
///
/// The entity cache holds repos from every imported project — each project's
/// stream prunes only its own scope — but the sidebar only ever shows the
/// **active** one, so repos outside it are dropped before grouping. Without
/// this, a previously-viewed project's repos linger in the sidebar after a
/// switch. No active project yields an empty tree.
#[must_use]
pub fn build_scoped_repo_tree(
    repos: &[RepoDTO],
    workspaces: &[WorkspaceDTO],
    active_project_id: Option<&str>,
) -> Vec<SidebarRepo> {
    let Some(active) = non_empty(active_project_id) else {
        return Vec::new();
    };
    repos
        .iter()
        .filter(|repo| repo.project_id == active)
        .map(|repo| to_sidebar_repo(repo, workspaces))
        .collect()
}

#[cfg(test)]
mod tests {
    use crowbar_proto::api_v0_dto::WorkspaceDTO;
    use crowbar_proto::domain::WorkspaceStatus;

    use super::{
        AvatarSource, build_repo_tree, build_scoped_repo_tree, to_sidebar_repo,
        to_sidebar_workspace,
    };
    use crate::sidebar::fixtures::{repo_dto, workspace_dto};

    #[test]
    fn carries_the_row_fields_through() {
        let dto = WorkspaceDTO {
            added: 12,
            deleted: 3,
            working: true,
            can_merge_locally: true,
            merge_conflicts: true,
            status: Some(WorkspaceStatus::PrOpen),
            parent_branch: Some("main".to_string()),
            pr_url: Some("https://example.invalid/pr/1".to_string()),
            ..workspace_dto("w1", "r1")
        };
        let row = to_sidebar_workspace(&dto);
        assert_eq!(row.added, 12);
        assert_eq!(row.deleted, 3);
        assert!(row.working);
        assert!(row.can_merge_locally);
        assert!(row.merge_conflicts);
        assert_eq!(row.status, Some(WorkspaceStatus::PrOpen));
        assert_eq!(row.parent_branch.as_deref(), Some("main"));
        assert_eq!(row.pr_url.as_deref(), Some("https://example.invalid/pr/1"));
    }

    /// `last_error` and `held_by_path` are spread unconditionally by the TS so
    /// a cleared value overwrites a stale one. An absent field must therefore
    /// become empty, never stay unset.
    #[test]
    fn absent_error_and_holder_become_empty_not_absent() {
        let row = to_sidebar_workspace(&workspace_dto("w1", "r1"));
        assert_eq!(row.last_error, "");
        assert_eq!(row.held_by_path, "");
    }

    #[test]
    fn empty_optional_strings_count_as_absent() {
        let dto = WorkspaceDTO {
            parent_id: Some(String::new()),
            pr_url: Some(String::new()),
            parent_branch: Some(String::new()),
            local_path: Some(String::new()),
            ..workspace_dto("w1", "r1")
        };
        let row = to_sidebar_workspace(&dto);
        assert_eq!(row.parent_id, None);
        assert_eq!(row.pr_url, None);
        assert_eq!(row.parent_branch, None);
        assert_eq!(row.local_path, None);
    }

    #[test]
    fn groups_only_this_repos_workspaces() {
        let repo = repo_dto("r1", "p1");
        let workspaces = vec![
            workspace_dto("w1", "r1"),
            workspace_dto("w2", "r2"),
            workspace_dto("w3", "r1"),
        ];
        let out = to_sidebar_repo(&repo, &workspaces);
        let ids: Vec<&str> = out.workspaces.iter().map(|w| w.id.as_str()).collect();
        assert_eq!(ids, ["w1", "w3"]);
    }

    /// The default workspace is the repo header, not a row — and three of its
    /// fields are lifted onto the repo because filtering it out would drop
    /// them.
    #[test]
    fn lifts_the_default_workspace_onto_the_repo() {
        let repo = repo_dto("r1", "p1");
        let default = WorkspaceDTO {
            is_default: Some(true),
            working: true,
            status: Some(WorkspaceStatus::Locked),
            branch: "main".to_string(),
            ..workspace_dto("w-default", "r1")
        };
        let out = to_sidebar_repo(&repo, &[default, workspace_dto("w1", "r1")]);

        let ids: Vec<&str> = out.workspaces.iter().map(|w| w.id.as_str()).collect();
        assert_eq!(ids, ["w1"], "the default workspace is not a tree row");
        assert_eq!(out.default_workspace_id.as_deref(), Some("w-default"));
        assert_eq!(out.default_branch.as_deref(), Some("main"));
        assert!(out.default_working, "its spinner drives the repo header");
        assert_eq!(out.default_status, Some(WorkspaceStatus::Locked));
    }

    #[test]
    fn a_repo_with_no_default_workspace_lifts_nothing() {
        let out = to_sidebar_repo(&repo_dto("r1", "p1"), &[workspace_dto("w1", "r1")]);
        assert_eq!(out.default_workspace_id, None);
        assert_eq!(out.default_branch, None);
        assert!(!out.default_working);
        assert_eq!(out.default_status, None);
    }

    #[test]
    fn avatar_prefers_emoji_then_icon_then_initials() {
        let emoji = crowbar_proto::api_v0_dto::RepoDTO {
            avatar_emoji: Some("\u{1f98a}".to_string()),
            avatar_url: Some("/v0/icon".to_string()),
            ..repo_dto("r1", "p1")
        };
        assert_eq!(
            AvatarSource::of(&emoji),
            AvatarSource::Emoji("\u{1f98a}".to_string()),
            "a custom emoji wins over a fetched icon"
        );

        let icon = crowbar_proto::api_v0_dto::RepoDTO {
            avatar_url: Some("/v0/icon".to_string()),
            ..repo_dto("r1", "p1")
        };
        assert_eq!(
            AvatarSource::of(&icon),
            AvatarSource::Icon("/v0/icon".to_string())
        );

        assert_eq!(
            AvatarSource::of(&repo_dto("r1", "p1")),
            AvatarSource::Initials
        );
    }

    #[test]
    fn empty_avatar_strings_fall_through_to_initials() {
        let repo = crowbar_proto::api_v0_dto::RepoDTO {
            avatar_emoji: Some(String::new()),
            avatar_url: Some(String::new()),
            ..repo_dto("r1", "p1")
        };
        assert_eq!(AvatarSource::of(&repo), AvatarSource::Initials);
    }

    /// The daemon's palette token is carried verbatim. It resolves to no
    /// background at all in the reference app, and that resolution is
    /// `crowbar-ui`'s — see the module docs.
    #[test]
    fn avatar_colour_is_carried_verbatim() {
        let out = to_sidebar_repo(&repo_dto("r1", "p1"), &[]);
        assert_eq!(out.avatar_color, "avatar-slate");
        assert_eq!(out.avatar_label, "R");
    }

    #[test]
    fn build_repo_tree_covers_every_repo() {
        let repos = vec![repo_dto("r1", "p1"), repo_dto("r2", "p1")];
        let workspaces = vec![workspace_dto("w1", "r1"), workspace_dto("w2", "r2")];
        let tree = build_repo_tree(&repos, &workspaces);
        assert_eq!(tree.len(), 2);
        assert_eq!(tree[0].workspaces.len(), 1);
        assert_eq!(tree[1].workspaces.len(), 1);
    }

    /// The cache is cross-project on purpose; the sidebar is not.
    #[test]
    fn scoped_tree_drops_other_projects_repos() {
        let repos = vec![repo_dto("r1", "p1"), repo_dto("r2", "p2")];
        let workspaces = vec![workspace_dto("w1", "r1"), workspace_dto("w2", "r2")];
        let tree = build_scoped_repo_tree(&repos, &workspaces, Some("p1"));
        assert_eq!(tree.len(), 1);
        assert_eq!(tree[0].id, "r1");
    }

    #[test]
    fn no_active_project_yields_an_empty_tree() {
        let repos = vec![repo_dto("r1", "p1")];
        assert!(build_scoped_repo_tree(&repos, &[], None).is_empty());
        assert!(build_scoped_repo_tree(&repos, &[], Some("")).is_empty());
    }

    #[test]
    fn repo_path_becomes_the_local_path_fallback() {
        let out = to_sidebar_repo(&repo_dto("r1", "p1"), &[]);
        assert_eq!(out.local_path.as_deref(), Some("/r/r1"));

        let pathless = crowbar_proto::api_v0_dto::RepoDTO {
            path: String::new(),
            ..repo_dto("r1", "p1")
        };
        assert_eq!(to_sidebar_repo(&pathless, &[]).local_path, None);
    }
}
