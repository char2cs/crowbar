//! `SidebarStore` — everything the sidebar renders from, in one entity.
//!
//! The `Entity<T>` half of `web/src/lib/store/sidebar.ts`,
//! `features/layout/stores/sidebar-*.ts` and `lib/store/workspace-list.ts`.
//! Every decision it makes lives in `crowbar_core::sidebar`; this type owns
//! the state those functions operate on, the derived tree, and when a rebuild
//! happens.
//!
//! # Deriving is coalesced, not per frame
//!
//! A repo with 200 workspaces must not re-derive 200 nodes because one
//! `working` flag flipped. Two things prevent it:
//!
//! * **Frames arrive in batches.** [`SidebarStore::apply_batch`] takes
//!   everything the stream had ready, applies it, and rebuilds **once**. The
//!   daemon emits a burst per poll, so this is the common case, not an
//!   optimisation for a rare one.
//! * **Per-repo subtrees are memoised.** A repo whose rows did not change
//!   keeps its existing nodes, exactly as React's `useMemo` on `rootsByRepo`
//!   does — a hover or a drag must not rebuild every repo's node graph.

use std::collections::HashMap;

use crowbar_core::sidebar::cache::{EntityCache, Generation, Scope, Seed};
use crowbar_core::sidebar::collapse::Collapsed;
use crowbar_core::sidebar::hierarchy::{Node, build_workspace_tree};
use crowbar_core::sidebar::panel::SidebarPanel;
use crowbar_core::sidebar::tabs::Tab;
use crowbar_core::sidebar::tree::{SidebarRepo, build_scoped_repo_tree};
use crowbar_core::workspace::scope::{WorkspaceScope, WorkspaceScopeRegistry};
use gpui::{App, AppContext as _, Context, Entity};

/// Whether the daemon's streams are live.
///
/// The React app keeps this in a store of its own, written by the WS
/// manager's `reportChannelState` and read by `connection-indicator.tsx`.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub enum Connection {
    /// No stream has reported yet.
    #[default]
    Connecting,
    /// Every open stream is up.
    Live,
    /// At least one stream is down, with the most recent reason.
    Down(String),
}

/// The sidebar's state.
pub struct SidebarStore {
    cache: EntityCache,
    repos: Vec<SidebarRepo>,
    trees: HashMap<String, Vec<Node>>,
    collapsed: Collapsed,
    active_tab: Tab,
    panel: SidebarPanel,
    scopes: WorkspaceScopeRegistry,
    active_project_id: Option<String>,
    /// Per-stream liveness, keyed by daemon path. Aggregated by
    /// [`Self::connection`] rather than stored as one flag: a workspace stream
    /// dropping while the project stream is fine is a different picture from
    /// the daemon being gone, and collapsing them early loses that.
    channels: HashMap<String, Result<(), String>>,
}

impl SidebarStore {
    /// A store with nothing loaded, restored from the daemon's two UI rows.
    #[must_use]
    pub fn new(
        stored_open: Option<&str>,
        stored_width: Option<&str>,
        stored_tab: Option<&str>,
    ) -> Self {
        Self {
            cache: EntityCache::new(),
            repos: Vec::new(),
            trees: HashMap::new(),
            collapsed: Collapsed::new(),
            active_tab: stored_tab.map_or_else(Tab::default, Tab::from_str_or_default),
            panel: SidebarPanel::restore(stored_open, stored_width),
            scopes: WorkspaceScopeRegistry::new(),
            active_project_id: None,
            channels: HashMap::new(),
        }
    }

    /// Create the store as an entity.
    pub fn build(
        cx: &mut App,
        stored_open: Option<&str>,
        stored_width: Option<&str>,
        stored_tab: Option<&str>,
    ) -> Entity<Self> {
        cx.new(|_| Self::new(stored_open, stored_width, stored_tab))
    }

    // --- reads -------------------------------------------------------------

    /// The repo sections, scoped to the active project.
    #[must_use]
    pub fn repos(&self) -> &[SidebarRepo] {
        &self.repos
    }

    /// One repo's nested workspace rows.
    #[must_use]
    pub fn tree_for(&self, repo_id: &str) -> &[Node] {
        self.trees.get(repo_id).map_or(&[], Vec::as_slice)
    }

    /// Which panel is showing.
    #[must_use]
    pub const fn active_tab(&self) -> Tab {
        self.active_tab
    }

    /// The panel's width and open state.
    #[must_use]
    pub const fn panel(&self) -> &SidebarPanel {
        &self.panel
    }

    /// Mutable access to the panel, for the resize and toggle handlers.
    pub const fn panel_mut(&mut self) -> &mut SidebarPanel {
        &mut self.panel
    }

    /// Which rows are folded shut.
    #[must_use]
    pub const fn collapsed(&self) -> &Collapsed {
        &self.collapsed
    }

    /// Mutable access to the collapse sets.
    pub const fn collapsed_mut(&mut self) -> &mut Collapsed {
        &mut self.collapsed
    }

    /// The scope the user is looking at: project, repo and workspace.
    ///
    /// `get_workspace_scope(None)` is the registry's own "the active one"
    /// query — the same call `workspaceBase()` makes when it is not given an
    /// explicit id.
    #[must_use]
    pub fn active_scope(&self) -> Option<&WorkspaceScope> {
        self.scopes.get_workspace_scope(None)
    }

    /// The workspace the user is looking at, if any.
    #[must_use]
    pub fn active_workspace_id(&self) -> Option<&str> {
        self.active_scope().map(|scope| scope.ws_id.as_str())
    }

    /// Resolve any workspace's scope, whether or not it is the active one.
    /// Every row's scope is recorded as the tree is built — see
    /// [`Self::rebuild`] — so an action on a never-visited row still resolves.
    #[must_use]
    pub fn scope_of(&self, ws_id: &str) -> Option<&WorkspaceScope> {
        self.scopes.get_workspace_scope(Some(ws_id))
    }

    /// The project whose repos the sidebar shows.
    #[must_use]
    pub fn active_project_id(&self) -> Option<&str> {
        self.active_project_id.as_deref()
    }

    /// Every project the daemon knows about, for the project switcher.
    #[must_use]
    pub fn projects(&self) -> Vec<crowbar_core::proto::api_v0_dto::ProjectDTO> {
        self.cache.projects()
    }

    /// Whether the daemon's streams are live.
    #[must_use]
    pub fn connection(&self) -> Connection {
        if self.channels.is_empty() {
            return Connection::Connecting;
        }
        self.channels
            .values()
            .find_map(|state| state.as_ref().err())
            .map_or(Connection::Live, |reason| Connection::Down(reason.clone()))
    }

    // --- selection ---------------------------------------------------------

    /// Make `ws_id` the active workspace.
    ///
    /// **This is the state the archive found missing.** `project-home-row`
    /// held a PASS verdict in its `selected` cell while the assembled shell
    /// rendered every row inactive, because nothing modelled which row was
    /// active. A per-component oracle cannot see that: the state is an input
    /// the harness supplies, so the cell always exists and the app that never
    /// produces it is out of frame.
    pub fn select_workspace(&mut self, scope: WorkspaceScope, cx: &mut Context<Self>) {
        self.scopes.set_workspace_scope(scope);
        cx.notify();
    }

    /// Switch the sidebar to another project.
    ///
    /// The old project's repos are dropped **immediately** rather than when
    /// the new seed lands, so they do not sit on screen belonging to a project
    /// the user has left. React does the same, in the same order, and the
    /// order is the point.
    pub fn set_active_project(&mut self, project_id: Option<String>, cx: &mut Context<Self>) {
        if self.active_project_id == project_id {
            return;
        }
        if let Some(previous) = self.active_project_id.take() {
            self.cache.forget_project(&previous);
        }
        self.active_project_id = project_id;
        self.rebuild();
        cx.notify();
    }

    /// Show a different carousel panel. Returns the value to persist.
    pub fn set_active_tab(&mut self, tab: Tab, cx: &mut Context<Self>) -> Option<&'static str> {
        if self.active_tab == tab {
            return None;
        }
        self.active_tab = tab;
        cx.notify();
        Some(tab.as_str())
    }

    // --- the stream --------------------------------------------------------

    /// Note that a stream came up.
    pub fn note_connected(&mut self, path: &str, cx: &mut Context<Self>) {
        self.channels.insert(path.to_owned(), Ok(()));
        cx.notify();
    }

    /// Note that a stream went down.
    pub fn note_disconnected(&mut self, path: &str, reason: String, cx: &mut Context<Self>) {
        self.channels.insert(path.to_owned(), Err(reason));
        cx.notify();
    }

    /// Forget a stream that is no longer open, so a torn-down scope does not
    /// hold the connection indicator red forever.
    pub fn forget_channel(&mut self, path: &str, cx: &mut Context<Self>) {
        self.channels.remove(path);
        cx.notify();
    }

    /// Start a reseed and take its ticket.
    pub fn begin_reseed(&mut self) -> Generation {
        self.cache.begin_reseed()
    }

    /// Apply a completed seed, rebuilding if it changed anything.
    pub fn apply_seed(
        &mut self,
        scope: &Scope,
        generation: Generation,
        seed: Seed,
        cx: &mut Context<Self>,
    ) {
        if self.cache.apply_seed(scope, generation, seed) {
            self.rebuild();
            cx.notify();
        }
    }

    /// Apply a batch of decoded frames, rebuilding **once** if any changed
    /// anything. See the module docs for why this is a batch.
    pub fn apply_batch(&mut self, frames: Vec<Decoded>, cx: &mut Context<Self>) {
        let mut changed = false;
        for frame in frames {
            changed |= match frame {
                Decoded::Project(dto) => self.cache.apply_project(dto),
                Decoded::Repo(dto) => self.cache.apply_repo(dto),
                Decoded::Workspace(dto) => self.cache.apply_workspace(dto),
            };
        }
        if changed {
            self.rebuild();
            cx.notify();
        }
    }

    /// Rebuild the derived tree from the cache.
    ///
    /// Per-repo subtrees are reused when the repo's rows are unchanged, which
    /// is what keeps an unrelated frame — one repo's spinner flipping — from
    /// rebuilding every other repo's node graph.
    fn rebuild(&mut self) {
        let repos = build_scoped_repo_tree(
            &self.cache.repos(),
            &self.cache.workspaces(),
            self.active_project_id.as_deref(),
        );

        let mut trees = HashMap::with_capacity(repos.len());
        for (index, repo) in repos.iter().enumerate() {
            let unchanged = self.repos.get(index).is_some_and(|previous| {
                previous.id == repo.id && previous.workspaces == repo.workspaces
            });
            let tree = if unchanged {
                self.trees.remove(&repo.id)
            } else {
                None
            };
            trees.insert(
                repo.id.clone(),
                tree.unwrap_or_else(|| build_workspace_tree(&repo.workspaces)),
            );
        }

        // Every workspace's scope is recorded, not just the one the user
        // navigated to. A scoped URL cannot be built for an unrecorded
        // workspace, so without this an action on a never-visited row —
        // Retry/Detach… on a placeholder — silently no-ops.
        for repo in &repos {
            for ws in &repo.workspaces {
                self.scopes.record_workspace_scope(WorkspaceScope {
                    project_id: repo.project_id.clone(),
                    repo_id: repo.id.clone(),
                    ws_id: ws.id.clone(),
                });
            }
            // The default workspace is not a tree row but is still navigable:
            // the repo header opens it.
            if let Some(default_id) = &repo.default_workspace_id {
                self.scopes.record_workspace_scope(WorkspaceScope {
                    project_id: repo.project_id.clone(),
                    repo_id: repo.id.clone(),
                    ws_id: default_id.clone(),
                });
            }
        }

        // A deleted-then-recreated id must not inherit the old row's fold.
        let repo_ids = repos.iter().map(|r| r.id.clone()).collect();
        let ws_ids = repos
            .iter()
            .flat_map(|r| r.workspaces.iter().map(|w| w.id.clone()))
            .collect();
        self.collapsed.retain_known(&repo_ids, &ws_ids);

        self.repos = repos;
        self.trees = trees;
    }
}

/// A stream frame, decoded to the DTO its scope carries.
#[derive(Debug, Clone, PartialEq)]
pub enum Decoded {
    /// From `/v0/projects`.
    Project(crowbar_core::proto::api_v0_dto::ProjectDTO),
    /// From `/v0/projects/:p/repos`.
    Repo(crowbar_core::proto::api_v0_dto::RepoDTO),
    /// From `/v0/projects/:p/repos/:r/workspaces`.
    Workspace(crowbar_core::proto::api_v0_dto::WorkspaceDTO),
}

#[cfg(test)]
mod tests {
    use crowbar_core::proto::api_v0_dto::{ProjectDTO, RepoDTO, WorkspaceDTO};
    use crowbar_core::proto::domain::WorkspaceStatus;
    use crowbar_core::proto::domain_git::MergeStrategy;
    use crowbar_core::sidebar::cache::{Scope, Seed};
    use crowbar_core::sidebar::tabs::Tab;
    use crowbar_core::workspace::scope::WorkspaceScope;
    use gpui::TestAppContext;

    use super::{Connection, Decoded, SidebarStore};

    fn workspace(id: &str, repo_id: &str) -> WorkspaceDTO {
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

    fn repo(id: &str, project_id: &str) -> RepoDTO {
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

    fn project(id: &str) -> ProjectDTO {
        ProjectDTO {
            id: id.to_string(),
            name: format!("project-{id}"),
            path: format!("/p/{id}"),
            status: None,
            last_activity: String::new(),
        }
    }

    /// A store already showing one repo with two workspaces under project p1.
    fn loaded(cx: &mut TestAppContext) -> gpui::Entity<SidebarStore> {
        cx.update(|cx| {
            let store = SidebarStore::build(cx, None, None, None);
            store.update(cx, |store, cx| {
                store.set_active_project(Some("p1".to_string()), cx);
                let generation = store.begin_reseed();
                store.apply_seed(
                    &Scope::Repos {
                        project_id: "p1".to_string(),
                    },
                    generation,
                    Seed::Repos(vec![repo("r1", "p1")]),
                    cx,
                );
                let generation = store.begin_reseed();
                store.apply_seed(
                    &Scope::Workspaces {
                        project_id: "p1".to_string(),
                        repo_id: "r1".to_string(),
                    },
                    generation,
                    Seed::Workspaces(vec![workspace("w1", "r1"), workspace("w2", "r1")]),
                    cx,
                );
            });
            store
        })
    }

    #[gpui::test]
    fn a_seed_becomes_a_rendered_tree(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = loaded(cx);

        store.read_with(cx, |store, _| {
            assert_eq!(store.repos().len(), 1);
            assert_eq!(store.repos()[0].id, "r1");
            assert_eq!(store.tree_for("r1").len(), 2);
        });
    }

    #[gpui::test]
    fn a_frame_updates_the_row_it_names(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = loaded(cx);

        store.update(cx, |store, cx| {
            store.apply_batch(
                vec![Decoded::Workspace(WorkspaceDTO {
                    working: true,
                    ..workspace("w1", "r1")
                })],
                cx,
            );
        });

        store.read_with(cx, |store, _| {
            let rows = &store.repos()[0].workspaces;
            assert!(
                rows.iter()
                    .find(|w| w.id == "w1")
                    .is_some_and(|w| w.working)
            );
            assert!(
                rows.iter()
                    .find(|w| w.id == "w2")
                    .is_some_and(|w| !w.working)
            );
        });
    }

    /// The derived tree is rebuilt, not appended to — a tombstone has to
    /// remove the row from what the sidebar renders, not only from the cache.
    #[gpui::test]
    fn a_tombstone_removes_the_row_from_the_tree(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = loaded(cx);

        store.update(cx, |store, cx| {
            store.apply_batch(
                vec![Decoded::Workspace(WorkspaceDTO {
                    status: Some(WorkspaceStatus::Deleted),
                    ..workspace("w1", "r1")
                })],
                cx,
            );
        });

        store.read_with(cx, |store, _| {
            assert_eq!(store.tree_for("r1").len(), 1);
            assert_eq!(store.repos()[0].workspaces[0].id, "w2");
        });
    }

    /// A repo whose rows did not change keeps its existing nodes. React
    /// memoises `rootsByRepo` for the same reason: a hover or a drag must not
    /// rebuild every repo's node graph.
    #[gpui::test]
    fn an_unrelated_frame_does_not_rebuild_a_repos_subtree(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = cx.update(|cx| {
            let store = SidebarStore::build(cx, None, None, None);
            store.update(cx, |store, cx| {
                store.set_active_project(Some("p1".to_string()), cx);
                let generation = store.begin_reseed();
                store.apply_seed(
                    &Scope::Repos {
                        project_id: "p1".to_string(),
                    },
                    generation,
                    Seed::Repos(vec![repo("r1", "p1"), repo("r2", "p1")]),
                    cx,
                );
                let generation = store.begin_reseed();
                store.apply_seed(
                    &Scope::Workspaces {
                        project_id: "p1".to_string(),
                        repo_id: "r1".to_string(),
                    },
                    generation,
                    Seed::Workspaces(vec![workspace("w1", "r1")]),
                    cx,
                );
                let generation = store.begin_reseed();
                store.apply_seed(
                    &Scope::Workspaces {
                        project_id: "p1".to_string(),
                        repo_id: "r2".to_string(),
                    },
                    generation,
                    Seed::Workspaces(vec![workspace("w9", "r2")]),
                    cx,
                );
            });
            store
        });

        let before = store.read_with(cx, |store, _| store.tree_for("r2").to_vec());

        store.update(cx, |store, cx| {
            store.apply_batch(
                vec![Decoded::Workspace(WorkspaceDTO {
                    working: true,
                    ..workspace("w1", "r1")
                })],
                cx,
            );
        });

        store.read_with(cx, |store, _| {
            assert_eq!(
                store.tree_for("r2"),
                before.as_slice(),
                "r2's nodes are untouched by a frame about r1"
            );
        });
    }

    /// **The state the archive found missing.** `project-home-row` held a PASS
    /// verdict in its `selected` cell while the assembled shell rendered every
    /// row inactive, because nothing modelled which row was active.
    #[gpui::test]
    fn selecting_a_workspace_is_modelled(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = loaded(cx);

        store.read_with(cx, |store, _| {
            assert_eq!(store.active_workspace_id(), None, "nothing is selected yet");
        });

        store.update(cx, |store, cx| {
            store.select_workspace(
                WorkspaceScope {
                    project_id: "p1".to_string(),
                    repo_id: "r1".to_string(),
                    ws_id: "w2".to_string(),
                },
                cx,
            );
        });

        store.read_with(cx, |store, _| {
            assert_eq!(store.active_workspace_id(), Some("w2"));
            assert_eq!(store.active_scope().map(|s| s.repo_id.as_str()), Some("r1"));
        });
    }

    /// Every row's scope is recorded as the tree is built, not just the one
    /// navigated to — without it an action on a never-visited row (Retry or
    /// Detach… on a placeholder) has no scoped URL and silently no-ops.
    #[gpui::test]
    fn every_rows_scope_is_resolvable_without_visiting_it(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = loaded(cx);

        store.read_with(cx, |store, _| {
            let scope = store.scope_of("w2").expect("recorded during rebuild");
            assert_eq!(scope.project_id, "p1");
            assert_eq!(scope.repo_id, "r1");
        });
    }

    /// The old project's repos leave the screen at once, not when the new
    /// seed lands.
    #[gpui::test]
    fn switching_project_drops_the_previous_ones_repos(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = loaded(cx);

        store.update(cx, |store, cx| {
            store.set_active_project(Some("p2".to_string()), cx);
        });

        store.read_with(cx, |store, _| {
            assert!(store.repos().is_empty());
            assert_eq!(store.active_project_id(), Some("p2"));
        });
    }

    #[gpui::test]
    fn projects_are_readable_for_the_switcher(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = cx.update(|cx| {
            let store = SidebarStore::build(cx, None, None, None);
            store.update(cx, |store, cx| {
                let generation = store.begin_reseed();
                store.apply_seed(
                    &Scope::Projects,
                    generation,
                    Seed::Projects(vec![project("p1"), project("p2")]),
                    cx,
                );
            });
            store
        });

        store.read_with(cx, |store, _| {
            assert_eq!(store.projects().len(), 2);
        });
    }

    // --- panel, tabs, connection ------------------------------------------

    #[gpui::test]
    fn the_panel_restores_from_the_daemons_rows(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store =
            cx.update(|cx| SidebarStore::build(cx, Some("false"), Some("400"), Some("git")));

        store.read_with(cx, |store, _| {
            assert!(!store.panel().is_open());
            assert!((store.panel().preferred_width() - 400.0).abs() < 1e-3);
            assert_eq!(store.active_tab(), Tab::Git);
        });
    }

    /// A preference row written by a newer build must not stop the sidebar
    /// from rendering.
    #[gpui::test]
    fn an_unknown_stored_tab_falls_back(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = cx.update(|cx| SidebarStore::build(cx, None, None, Some("search")));
        store.read_with(cx, |store, _| {
            assert_eq!(store.active_tab(), Tab::Workspaces);
        });
    }

    #[gpui::test]
    fn changing_tab_reports_what_to_persist(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));

        store.update(cx, |store, cx| {
            assert_eq!(store.set_active_tab(Tab::Files, cx), Some("files"));
            assert_eq!(
                store.set_active_tab(Tab::Files, cx),
                None,
                "an unchanged tab writes nothing"
            );
        });
    }

    /// One stream down is not the same picture as the daemon being gone, so
    /// liveness is per path and aggregated on read.
    #[gpui::test]
    fn connection_state_aggregates_across_streams(cx: &mut TestAppContext) {
        crowbar_driver::leak_checked!(cx);
        let store = cx.update(|cx| SidebarStore::build(cx, None, None, None));

        store.read_with(cx, |store, _| {
            assert_eq!(store.connection(), Connection::Connecting);
        });

        store.update(cx, |store, cx| {
            store.note_connected("/v0/projects", cx);
            store.note_connected("/v0/projects/p1/repos", cx);
        });
        store.read_with(cx, |store, _| {
            assert_eq!(store.connection(), Connection::Live);
        });

        store.update(cx, |store, cx| {
            store.note_disconnected("/v0/projects/p1/repos", "socket closed".into(), cx);
        });
        store.read_with(cx, |store, _| {
            assert_eq!(
                store.connection(),
                Connection::Down("socket closed".to_string())
            );
        });

        // A torn-down scope must not hold the indicator red forever.
        store.update(cx, |store, cx| {
            store.forget_channel("/v0/projects/p1/repos", cx);
        });
        store.read_with(cx, |store, _| {
            assert_eq!(store.connection(), Connection::Live);
        });
    }
}
