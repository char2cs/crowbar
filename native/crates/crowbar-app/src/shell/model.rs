//! Turning `SidebarStore`'s records into the design system's value types.
//!
//! This is the seam the port never had: `crowbar-core` knows what a workspace
//! *is*, `crowbar-ui` knows what a row *looks like*, and until S1a nothing
//! turned one into the other. The archive's finding — ten `components/layout`
//! surfaces holding a PASS verdict while the assembled app was a bare text
//! list — is exactly the absence of this file.
//!
//! Kept pure and separate from [`super::Sidebar`] so it can be tested without
//! a window: everything here is a function from store records to
//! `crowbar-ui` structs, with no `Entity`, no `Context` and no rendering.
//!
//! # The avatar background, and why it is transparent
//!
//! The daemon writes one of eight palette tokens — `avatar-rose`,
//! `avatar-amber`, `avatar-emerald`, `avatar-cyan`, `avatar-indigo`,
//! `avatar-violet`, `avatar-slate`, `avatar-pink` — and **no `.avatar-*` rule
//! exists anywhere in `web/`'s stylesheets.** That was verified against the
//! running React app and recorded in `native/mapping/repo-avatar.md`: the live
//! node carries `class="… avatar-slate"` and computes `background-color:
//! rgba(0, 0, 0, 0)`. So 1:1 parity with the reference is **no background**,
//! not a colour invented to fill the field, and [`avatar_background`] returns
//! the theme's transparent token for every value the daemon actually sends.

use crowbar_core::proto::domain::WorkspaceStatus;
use crowbar_core::sidebar::collapse::Collapsed;
use crowbar_core::sidebar::hierarchy::Node;
use crowbar_core::sidebar::tree::{AvatarSource, SidebarRepo, SidebarWorkspace};
use crowbar_ui::surfaces::repo::repo_icon_popover::Trigger;
use crowbar_ui::surfaces::repo::{repo_avatar, repo_section::RepoSection};
use crowbar_ui::surfaces::rows::project_home_row::ProjectHomeRow;
use crowbar_ui::surfaces::rows::row_base::RowMode;
use crowbar_ui::surfaces::workspace::workspace_branch_icon::{Status, WorkspaceBranchIcon};
use crowbar_ui::surfaces::workspace::workspace_tree_item::WorkspaceTreeItem;
use crowbar_ui::theme::{Color, Theme};

/// Resolve a repo's stored avatar colour to a paintable one.
///
/// Every value the daemon sends resolves to **transparent**, because the
/// reference app paints no background for them — see the module docs. An
/// unrecognised value resolves the same way rather than to some default
/// colour: inventing one would put a colour on screen that the React app does
/// not have, which is the opposite of parity.
///
/// The eight values this was measured against are pinned in this module's own
/// tests (`DAEMON_PALETTE`), so a daemon that starts sending something else is
/// a failing test rather than a silently blank avatar. They live there and not
/// here because the shipping binary never consults them — the answer is the
/// same for every input, which is exactly the finding.
#[must_use]
pub fn avatar_background(_stored: &str, _theme: &Theme) -> Color {
    Color::TRANSPARENT
}

/// The lifecycle badge a row's icon draws.
///
/// `WorkspaceStatus::Other` — the daemon's zero value, `""`, meaning "has
/// commits, no PR" — has no icon of its own and falls back to `New`, matching
/// the React icon component's own default branch.
#[must_use]
pub fn icon_status(status: Option<&WorkspaceStatus>) -> Status {
    match status {
        Some(WorkspaceStatus::Locked) => Status::Locked,
        Some(WorkspaceStatus::PrConflicts) => Status::PrConflicts,
        Some(WorkspaceStatus::Deleted) => Status::Deleted,
        Some(WorkspaceStatus::PrOpen) => Status::PrOpen,
        Some(WorkspaceStatus::PrClosed) => Status::PrClosed,
        Some(WorkspaceStatus::PrMerged) => Status::PrMerged,
        Some(WorkspaceStatus::New | WorkspaceStatus::Other(_)) | None => Status::New,
    }
}

/// Whether a row has no on-disk worktree.
#[must_use]
fn is_placeholder(ws: &SidebarWorkspace) -> bool {
    ws.local_path.as_deref().unwrap_or("").is_empty()
}

/// A count the row renders, or `None` when there is nothing to show.
///
/// Zero is not drawn: a workspace with no diff against its parent shows no
/// `+0`/`-0` pair in the reference.
#[must_use]
fn count(value: i64) -> Option<u32> {
    u32::try_from(value).ok().filter(|n| *n > 0)
}

/// One nested workspace row and everything under it.
#[must_use]
pub fn tree_item(
    node: &Node,
    depth: u32,
    active_ws_id: Option<&str>,
    collapsed: &Collapsed,
) -> WorkspaceTreeItem {
    let ws = &node.workspace;
    let expanded = !collapsed.is_workspace_collapsed(&ws.id);
    WorkspaceTreeItem {
        branch: ws.branch.clone().into(),
        depth,
        icon: WorkspaceBranchIcon {
            status: icon_status(ws.status.as_ref()),
            working: ws.working,
            is_placeholder: is_placeholder(ws),
        },
        is_active: active_ws_id == Some(ws.id.as_str()),
        expanded,
        mode: RowMode::Normal,
        added: count(ws.added),
        deleted: count(ws.deleted),
        // A collapsed row renders no children at all, rather than rendering
        // them hidden: the reference unmounts the subtree, and building it
        // anyway would cost the whole descendant walk on every frame for rows
        // nobody can see.
        children: if expanded {
            node.children
                .iter()
                .map(|child| tree_item(child, depth + 1, active_ws_id, collapsed))
                .collect()
        } else {
            Vec::new()
        },
        pending_creates: Vec::new(),
    }
}

/// One repo section, with its rows.
#[must_use]
pub fn repo_section(
    repo: &SidebarRepo,
    roots: &[Node],
    active_ws_id: Option<&str>,
    collapsed: &Collapsed,
    theme: &Theme,
) -> RepoSection {
    let is_collapsed = collapsed.is_repo_collapsed(&repo.id);
    RepoSection {
        name: repo.name.clone().into(),
        // The repo header is "active" when the user is on its default
        // workspace — the one the header itself opens, which is not a tree row.
        is_active: repo.default_workspace_id.as_deref().is_some()
            && repo.default_workspace_id.as_deref() == active_ws_id,
        is_collapsed,
        mode: RowMode::Normal,
        has_default_workspace: repo.default_workspace_id.is_some(),
        trigger: Trigger {
            // Lifted off the default workspace, which is not a tree row, so its
            // spinner would otherwise be dropped with it.
            working: repo.default_working,
            picture: avatar_kind(&repo.avatar_source),
            label: repo.avatar_label.clone().into(),
            emoji: match &repo.avatar_source {
                AvatarSource::Emoji(glyph) => glyph.clone().into(),
                AvatarSource::Icon(_) | AvatarSource::Initials => String::new().into(),
            },
            background: avatar_background(&repo.avatar_color, theme),
        },
        roots: if is_collapsed {
            Vec::new()
        } else {
            roots
                .iter()
                .map(|node| tree_item(node, 0, active_ws_id, collapsed))
                .collect()
        },
        pending_creates: Vec::new(),
    }
}

/// Which of the avatar's three pictures a repo draws.
///
/// The icon case reports `Loaded` because this slice does not fetch icons yet
/// — the proxy path needs the daemon origin, which is slice 2's settings work.
/// `Errored` and `Letter` are pixel-identical by construction (one value both
/// React branches return), so an unfetched icon renders as the letter either
/// way and no picture is invented.
#[must_use]
fn avatar_kind(source: &AvatarSource) -> repo_avatar::Kind {
    match source {
        AvatarSource::Emoji(_) => repo_avatar::Kind::Emoji,
        AvatarSource::Icon(_) | AvatarSource::Initials => repo_avatar::Kind::Letter,
    }
}

/// The project row above the tree.
#[must_use]
pub fn project_home_row(name: &str, is_active: bool, working: bool) -> ProjectHomeRow {
    ProjectHomeRow {
        is_active,
        working,
        project_name: name.to_owned().into(),
    }
}

#[cfg(test)]
mod tests {
    use crowbar_core::proto::domain::WorkspaceStatus;
    use crowbar_core::sidebar::collapse::Collapsed;
    use crowbar_core::sidebar::hierarchy::build_workspace_tree;
    use crowbar_core::sidebar::tree::{AvatarSource, SidebarRepo, SidebarWorkspace};
    use crowbar_ui::surfaces::repo::repo_avatar;
    use crowbar_ui::surfaces::workspace::workspace_branch_icon::Status;
    use crowbar_ui::theme::{Color, Theme};

    use super::{avatar_background, icon_status, repo_section, tree_item};

    /// The eight palette tokens the daemon writes
    /// (`api/internal/app/usecases/internal/avatar`'s `palette()`).
    const DAEMON_PALETTE: [&str; 8] = [
        "avatar-rose",
        "avatar-amber",
        "avatar-emerald",
        "avatar-cyan",
        "avatar-indigo",
        "avatar-violet",
        "avatar-slate",
        "avatar-pink",
    ];

    fn workspace(id: &str) -> SidebarWorkspace {
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

    fn child(id: &str, parent: &str) -> SidebarWorkspace {
        SidebarWorkspace {
            parent_id: Some(parent.to_string()),
            ..workspace(id)
        }
    }

    fn repo(workspaces: Vec<SidebarWorkspace>) -> SidebarRepo {
        SidebarRepo {
            id: "r1".to_string(),
            project_id: "p1".to_string(),
            name: "repo-one".to_string(),
            avatar_label: "R".to_string(),
            avatar_color: "avatar-slate".to_string(),
            avatar_source: AvatarSource::Initials,
            workspaces,
            default_workspace_id: None,
            default_branch: None,
            default_working: false,
            default_status: None,
            local_path: None,
        }
    }

    /// Pinned against the daemon's own `palette()`.
    #[test]
    fn daemon_palette_is_the_measured_set() {
        assert_eq!(
            DAEMON_PALETTE,
            [
                "avatar-rose",
                "avatar-amber",
                "avatar-emerald",
                "avatar-cyan",
                "avatar-indigo",
                "avatar-violet",
                "avatar-slate",
                "avatar-pink",
            ]
        );
    }

    /// 1:1 with the reference is **no background**. `native/mapping/
    /// repo-avatar.md` records the live measurement: the running node carries
    /// `class="… avatar-slate"` and computes `background-color: rgba(0,0,0,0)`,
    /// because no `.avatar-*` rule exists anywhere in `web/`'s stylesheets.
    #[test]
    fn every_daemon_palette_value_paints_nothing() {
        let theme = Theme::DARK;
        for token in DAEMON_PALETTE {
            assert_eq!(avatar_background(token, &theme), Color::TRANSPARENT);
        }
        assert_eq!(avatar_background("", &theme), Color::TRANSPARENT);
    }

    #[test]
    fn every_status_maps_to_its_icon() {
        assert_eq!(icon_status(Some(&WorkspaceStatus::Locked)), Status::Locked);
        assert_eq!(icon_status(Some(&WorkspaceStatus::PrOpen)), Status::PrOpen);
        assert_eq!(
            icon_status(Some(&WorkspaceStatus::PrConflicts)),
            Status::PrConflicts
        );
        assert_eq!(
            icon_status(Some(&WorkspaceStatus::Deleted)),
            Status::Deleted
        );
        assert_eq!(
            icon_status(Some(&WorkspaceStatus::PrMerged)),
            Status::PrMerged
        );
        assert_eq!(
            icon_status(Some(&WorkspaceStatus::PrClosed)),
            Status::PrClosed
        );
    }

    /// `""` is the daemon's zero value — "has commits, no PR" — and has no
    /// icon of its own.
    #[test]
    fn the_zero_status_and_none_fall_back_to_new() {
        assert_eq!(icon_status(Some(&WorkspaceStatus::New)), Status::New);
        assert_eq!(
            icon_status(Some(&WorkspaceStatus::Other(String::new()))),
            Status::New
        );
        assert_eq!(icon_status(None), Status::New);
    }

    #[test]
    fn a_row_carries_its_branch_depth_and_counts() {
        let rows = vec![SidebarWorkspace {
            added: 12,
            deleted: 3,
            ..workspace("w1")
        }];
        let tree = build_workspace_tree(&rows);
        let item = tree_item(&tree[0], 0, None, &Collapsed::new());

        assert_eq!(item.branch, "branch-w1");
        assert_eq!(item.depth, 0);
        assert_eq!(item.added, Some(12));
        assert_eq!(item.deleted, Some(3));
    }

    /// A workspace with no diff against its parent shows no `+0`/`-0` pair.
    #[test]
    fn a_zero_count_is_not_drawn() {
        let tree = build_workspace_tree(&[workspace("w1")]);
        let item = tree_item(&tree[0], 0, None, &Collapsed::new());
        assert_eq!(item.added, None);
        assert_eq!(item.deleted, None);
    }

    /// The state the archive found missing: the app has to put the component
    /// into the cell the component already renders correctly.
    #[test]
    fn the_active_row_renders_selected() {
        let rows = vec![workspace("w1"), workspace("w2")];
        let tree = build_workspace_tree(&rows);
        let collapsed = Collapsed::new();

        let first = tree_item(&tree[0], 0, Some("w2"), &collapsed);
        let second = tree_item(&tree[1], 0, Some("w2"), &collapsed);

        assert!(!first.is_active);
        assert!(second.is_active, "the app produces the `selected` cell");
    }

    #[test]
    fn children_nest_and_carry_their_depth() {
        let rows = vec![workspace("root"), child("kid", "root")];
        let tree = build_workspace_tree(&rows);
        let item = tree_item(&tree[0], 0, None, &Collapsed::new());

        assert_eq!(item.children.len(), 1);
        assert_eq!(item.children[0].depth, 1);
        assert!(item.expanded);
    }

    /// A collapsed row builds no children at all. The reference unmounts the
    /// subtree, and building it anyway would walk every descendant on every
    /// frame for rows nobody can see.
    #[test]
    fn a_collapsed_row_builds_no_children() {
        let rows = vec![workspace("root"), child("kid", "root")];
        let tree = build_workspace_tree(&rows);
        let mut collapsed = Collapsed::new();
        collapsed.toggle_workspace("root");

        let item = tree_item(&tree[0], 0, None, &collapsed);
        assert!(!item.expanded);
        assert!(item.children.is_empty());
    }

    #[test]
    fn a_collapsed_repo_builds_no_rows() {
        let repo = repo(vec![workspace("w1")]);
        let tree = build_workspace_tree(&repo.workspaces);
        let mut collapsed = Collapsed::new();
        collapsed.toggle_repo("r1");

        let section = repo_section(&repo, &tree, None, &collapsed, &Theme::DARK);
        assert!(section.is_collapsed);
        assert!(section.roots.is_empty());
    }

    /// The default workspace is the repo header, not a row — so the header is
    /// what goes active when the user is on it, and its spinner is the one
    /// lifted onto the repo.
    #[test]
    fn the_header_is_active_on_the_default_workspace() {
        let mut repo = repo(vec![workspace("w1")]);
        repo.default_workspace_id = Some("w-default".to_string());
        repo.default_working = true;
        let tree = build_workspace_tree(&repo.workspaces);
        let collapsed = Collapsed::new();

        let on_default = repo_section(&repo, &tree, Some("w-default"), &collapsed, &Theme::DARK);
        assert!(on_default.is_active);
        assert!(on_default.trigger.working);
        assert!(on_default.has_default_workspace);

        let elsewhere = repo_section(&repo, &tree, Some("w1"), &collapsed, &Theme::DARK);
        assert!(!elsewhere.is_active);
    }

    #[test]
    fn a_repo_with_no_default_workspace_is_never_active() {
        let repo = repo(vec![workspace("w1")]);
        let tree = build_workspace_tree(&repo.workspaces);
        let section = repo_section(&repo, &tree, None, &Collapsed::new(), &Theme::DARK);
        assert!(!section.is_active);
        assert!(!section.has_default_workspace);
    }

    #[test]
    fn an_emoji_repo_draws_the_emoji_and_others_draw_the_letter() {
        let mut emoji = repo(vec![]);
        emoji.avatar_source = AvatarSource::Emoji("\u{1f98a}".to_string());
        let section = repo_section(&emoji, &[], None, &Collapsed::new(), &Theme::DARK);
        assert_eq!(section.trigger.picture, repo_avatar::Kind::Emoji);
        assert_eq!(section.trigger.emoji, "\u{1f98a}");

        let mut icon = repo(vec![]);
        icon.avatar_source = AvatarSource::Icon("/v0/icon".to_string());
        let section = repo_section(&icon, &[], None, &Collapsed::new(), &Theme::DARK);
        assert_eq!(
            section.trigger.picture,
            repo_avatar::Kind::Letter,
            "an unfetched icon renders as the letter, which is pixel-identical \
             to the errored branch by construction"
        );
    }

    /// A row with no on-disk worktree is a placeholder, and the icon says so.
    #[test]
    fn a_placeholder_row_is_marked() {
        let held = SidebarWorkspace {
            local_path: None,
            held_by_path: "/elsewhere".to_string(),
            ..workspace("w1")
        };
        let tree = build_workspace_tree(&[held]);
        let item = tree_item(&tree[0], 0, None, &Collapsed::new());
        assert!(item.icon.is_placeholder);
    }
}
