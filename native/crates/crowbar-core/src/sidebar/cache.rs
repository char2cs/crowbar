//! The daemon's entity cache: complete DTOs, merged by id, keyed by scope.
//!
//! Ported from the cache half of `web/src/lib/ws/entity-stream.ts` and
//! `web/src/lib/persistence/entity-cache.ts`.
//!
//! # There is no persistence here, and that is the port decision
//!
//! React writes these DTOs to `IndexedDB` so a page reload survives. A native
//! process has no reload, and `native/QUEUE.md` records that same browser
//! cache defeating `CROWBAR_HOME` isolation, because the origin is shared
//! between dev instances — two Crowbars pointed at different daemons read one
//! another's rows. This cache lives in memory and dies with the process; a
//! restart re-seeds over HTTP.
//!
//! What is kept is the *discipline* the persistent cache needed:
//!
//! * **Complete DTOs, merged by id.** A frame is a whole record, not a delta,
//!   so applying one is an insert-or-replace and never a field merge.
//! * **Tombstones remove.** A frame whose status is `deleted` is the daemon
//!   saying the row is gone; there is no separate delete channel.
//! * **A reseed prunes, but only within its own scope.** See [`Scope`].
//! * **Seed generations.** A reseed started while an older one is still in
//!   flight must win, or the sidebar settles on the stale answer.

use std::collections::HashMap;

use crowbar_proto::api_v0_dto::{ProjectDTO, RepoDTO, WorkspaceDTO};
use crowbar_proto::domain::WorkspaceStatus;

/// The tombstone marker the daemon sets on a removal frame.
const DELETED: &str = "deleted";

/// One subscribable slice of the daemon's data.
///
/// A scope is both *what to ask for* — [`Scope::path`], which the daemon
/// dual-serves as a GET and a WebSocket upgrade — and *what a reseed of it is
/// allowed to prune*.
///
/// # The prune predicate is not an optimisation
///
/// `crowbar_workspaces` is fed by **two** streams: the per-repo list, whose
/// seed is all of a repo's workspaces, and the per-workspace stream, whose
/// seed is one. Treating either as authoritative over the whole store means
/// the narrow one deletes every sibling — in the React app, navigating into
/// one workspace emptied the sidebar of all the others until a reload. Each
/// scope prunes only rows it could have seen.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Scope {
    /// Every project.
    Projects,
    /// Every repo in one project.
    Repos {
        /// The owning project.
        project_id: String,
    },
    /// Every workspace in one repo.
    Workspaces {
        /// The owning project.
        project_id: String,
        /// The owning repo.
        repo_id: String,
    },
}

impl Scope {
    /// The daemon path this scope seeds from and subscribes to.
    #[must_use]
    pub fn path(&self) -> String {
        match self {
            Self::Projects => "/v0/projects".to_owned(),
            Self::Repos { project_id } => format!("/v0/projects/{project_id}/repos"),
            Self::Workspaces {
                project_id,
                repo_id,
            } => format!("/v0/projects/{project_id}/repos/{repo_id}/workspaces"),
        }
    }
}

/// Complete DTOs from the daemon, by id.
#[derive(Debug, Clone, Default, PartialEq)]
pub struct EntityCache {
    projects: HashMap<String, ProjectDTO>,
    repos: HashMap<String, RepoDTO>,
    workspaces: HashMap<String, WorkspaceDTO>,
    generation: u64,
}

impl EntityCache {
    /// An empty cache.
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// Every project, in id order.
    ///
    /// Sorted rather than left in hash order so the tree the sidebar derives
    /// is stable across rebuilds: an unordered map would reshuffle rows on
    /// every frame, which reads as flicker and defeats any diffing above.
    #[must_use]
    pub fn projects(&self) -> Vec<ProjectDTO> {
        sorted(&self.projects)
    }

    /// Every repo, in id order.
    #[must_use]
    pub fn repos(&self) -> Vec<RepoDTO> {
        sorted(&self.repos)
    }

    /// Every workspace, in id order.
    #[must_use]
    pub fn workspaces(&self) -> Vec<WorkspaceDTO> {
        sorted(&self.workspaces)
    }

    /// Insert or replace a project, or remove it if the frame is a tombstone.
    /// Returns whether anything changed.
    pub fn apply_project(&mut self, dto: ProjectDTO) -> bool {
        if dto.status.as_deref() == Some(DELETED) {
            return self.projects.remove(&dto.id).is_some();
        }
        upsert(&mut self.projects, dto.id.clone(), dto)
    }

    /// Insert or replace a repo, or remove it if the frame is a tombstone.
    pub fn apply_repo(&mut self, dto: RepoDTO) -> bool {
        if dto.status.as_deref() == Some(DELETED) {
            return self.repos.remove(&dto.id).is_some();
        }
        upsert(&mut self.repos, dto.id.clone(), dto)
    }

    /// Insert or replace a workspace, or remove it if the frame is a
    /// tombstone.
    pub fn apply_workspace(&mut self, dto: WorkspaceDTO) -> bool {
        if dto.status == Some(WorkspaceStatus::Deleted) {
            return self.workspaces.remove(&dto.id).is_some();
        }
        upsert(&mut self.workspaces, dto.id.clone(), dto)
    }

    /// Begin a reseed, taking the generation it must still be current at when
    /// its GET returns.
    ///
    /// A reseed is not atomic: the GET is in flight for as long as the daemon
    /// takes. If a second reseed starts meanwhile — a reconnect during a slow
    /// seed — the first one's answer is older than the second's and must be
    /// discarded, or the cache settles on the stale set.
    pub fn begin_reseed(&mut self) -> Generation {
        self.generation += 1;
        Generation(self.generation)
    }

    /// Whether a reseed started at `generation` is still the current one.
    #[must_use]
    pub fn is_current(&self, generation: Generation) -> bool {
        generation.0 == self.generation
    }

    /// Apply a completed seed for `scope`: prune the rows this scope owns that
    /// the seed did not mention, then insert everything it did.
    ///
    /// Returns `false` without touching the cache when `generation` has been
    /// superseded.
    pub fn apply_seed(&mut self, scope: &Scope, generation: Generation, seed: Seed) -> bool {
        if !self.is_current(generation) {
            return false;
        }
        match (scope, seed) {
            (Scope::Projects, Seed::Projects(items)) => {
                let fresh: Vec<String> = items.iter().map(|p| p.id.clone()).collect();
                self.projects.retain(|id, _| fresh.contains(id));
                for item in items {
                    self.apply_project(item);
                }
                true
            }
            (Scope::Repos { project_id }, Seed::Repos(items)) => {
                let fresh: Vec<String> = items.iter().map(|r| r.id.clone()).collect();
                self.repos
                    .retain(|id, repo| repo.project_id != *project_id || fresh.contains(id));
                for item in items {
                    self.apply_repo(item);
                }
                true
            }
            (Scope::Workspaces { repo_id, .. }, Seed::Workspaces(items)) => {
                let fresh: Vec<String> = items.iter().map(|w| w.id.clone()).collect();
                self.workspaces
                    .retain(|id, ws| ws.repo_id != *repo_id || fresh.contains(id));
                for item in items {
                    self.apply_workspace(item);
                }
                true
            }
            // A seed of the wrong kind for its scope is a programming error at
            // the call site, not a daemon condition. It is dropped rather than
            // panicked on: an unreachable arm that aborts the app is worse
            // than one that leaves the cache as it was.
            _ => false,
        }
    }

    /// Drop everything a project owns. Used when a project stops being the
    /// active one, so its repos do not linger in a sidebar scoped elsewhere.
    pub fn forget_project(&mut self, project_id: &str) {
        self.repos.retain(|_, repo| repo.project_id != project_id);
        self.workspaces.retain(|_, ws| ws.project_id != project_id);
    }
}

/// A reseed's payload, typed to its scope.
#[derive(Debug, Clone, PartialEq)]
pub enum Seed {
    /// The answer to `GET /v0/projects`.
    Projects(Vec<ProjectDTO>),
    /// The answer to `GET /v0/projects/:p/repos`.
    Repos(Vec<RepoDTO>),
    /// The answer to `GET /v0/projects/:p/repos/:r/workspaces`.
    Workspaces(Vec<WorkspaceDTO>),
}

/// A reseed's ticket. See [`EntityCache::begin_reseed`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Generation(u64);

/// Insert or replace, reporting whether the stored value actually changed.
///
/// The daemon re-broadcasts unchanged records — a poll that found nothing new
/// still emits — so treating every frame as a change would rebuild the whole
/// derived tree for nothing, many times a second.
fn upsert<T: PartialEq>(map: &mut HashMap<String, T>, id: String, value: T) -> bool {
    match map.get(&id) {
        Some(existing) if *existing == value => false,
        _ => {
            map.insert(id, value);
            true
        }
    }
}

/// Values in key order. The key **is** the DTO's id, by construction — every
/// insert here is `map.insert(dto.id.clone(), dto)` — so sorting on it needs
/// no extractor and cannot disagree with the record it orders.
fn sorted<T: Clone>(map: &HashMap<String, T>) -> Vec<T> {
    let mut pairs: Vec<(&String, &T)> = map.iter().collect();
    pairs.sort_by(|a, b| a.0.cmp(b.0));
    pairs.into_iter().map(|(_, value)| value.clone()).collect()
}

#[cfg(test)]
mod tests {
    use crowbar_proto::api_v0_dto::{ProjectDTO, RepoDTO, WorkspaceDTO};
    use crowbar_proto::domain::WorkspaceStatus;

    use super::{EntityCache, Scope, Seed};
    use crate::sidebar::fixtures::{repo_dto, workspace_dto};

    fn project_dto(id: &str) -> ProjectDTO {
        ProjectDTO {
            id: id.to_string(),
            name: format!("project-{id}"),
            path: format!("/p/{id}"),
            status: None,
            last_activity: String::new(),
        }
    }

    fn ws_in(id: &str, project_id: &str, repo_id: &str) -> WorkspaceDTO {
        WorkspaceDTO {
            project_id: project_id.to_string(),
            ..workspace_dto(id, repo_id)
        }
    }

    fn ids<T>(items: &[T], id: impl Fn(&T) -> &str) -> Vec<&str> {
        items.iter().map(id).collect()
    }

    // --- paths -------------------------------------------------------------

    /// One path per scope, dual-served: the GET seeds it and the upgrade
    /// streams it.
    #[test]
    fn each_scope_names_its_daemon_path() {
        assert_eq!(Scope::Projects.path(), "/v0/projects");
        assert_eq!(
            Scope::Repos {
                project_id: "p1".into()
            }
            .path(),
            "/v0/projects/p1/repos"
        );
        assert_eq!(
            Scope::Workspaces {
                project_id: "p1".into(),
                repo_id: "r1".into(),
            }
            .path(),
            "/v0/projects/p1/repos/r1/workspaces"
        );
    }

    // --- merging -----------------------------------------------------------

    #[test]
    fn a_frame_inserts_then_replaces_by_id() {
        let mut cache = EntityCache::new();
        assert!(cache.apply_workspace(workspace_dto("w1", "r1")));
        assert_eq!(cache.workspaces().len(), 1);

        let updated = WorkspaceDTO {
            working: true,
            ..workspace_dto("w1", "r1")
        };
        assert!(cache.apply_workspace(updated));
        assert_eq!(cache.workspaces().len(), 1, "replaced, not appended");
        assert!(cache.workspaces()[0].working);
    }

    /// The daemon re-broadcasts unchanged records — a poll that found nothing
    /// still emits — so an unchanged frame must not report a change, or the
    /// derived tree rebuilds many times a second for nothing.
    #[test]
    fn an_identical_frame_reports_no_change() {
        let mut cache = EntityCache::new();
        assert!(cache.apply_workspace(workspace_dto("w1", "r1")));
        assert!(
            !cache.apply_workspace(workspace_dto("w1", "r1")),
            "the second frame carries the same record"
        );
        assert!(!cache.apply_repo(repo_dto("r1", "p1")) || cache.repos().len() == 1);
    }

    #[test]
    fn a_tombstone_removes_the_row() {
        let mut cache = EntityCache::new();
        cache.apply_workspace(workspace_dto("w1", "r1"));
        let tombstone = WorkspaceDTO {
            status: Some(WorkspaceStatus::Deleted),
            ..workspace_dto("w1", "r1")
        };
        assert!(cache.apply_workspace(tombstone));
        assert!(cache.workspaces().is_empty());
    }

    #[test]
    fn a_tombstone_for_an_unknown_row_reports_no_change() {
        let mut cache = EntityCache::new();
        let tombstone = WorkspaceDTO {
            status: Some(WorkspaceStatus::Deleted),
            ..workspace_dto("never-seen", "r1")
        };
        assert!(!cache.apply_workspace(tombstone));
    }

    #[test]
    fn project_and_repo_tombstones_use_the_string_marker() {
        let mut cache = EntityCache::new();
        cache.apply_project(project_dto("p1"));
        cache.apply_repo(repo_dto("r1", "p1"));

        assert!(cache.apply_project(ProjectDTO {
            status: Some("deleted".into()),
            ..project_dto("p1")
        }));
        assert!(cache.apply_repo(RepoDTO {
            status: Some("deleted".into()),
            ..repo_dto("r1", "p1")
        }));

        assert!(cache.projects().is_empty());
        assert!(cache.repos().is_empty());
    }

    /// Hash order would reshuffle rows on every rebuild, which reads as
    /// flicker and defeats any diffing above this layer.
    #[test]
    fn reads_come_back_in_a_stable_order() {
        let mut cache = EntityCache::new();
        for id in ["w3", "w1", "w2"] {
            cache.apply_workspace(workspace_dto(id, "r1"));
        }
        assert_eq!(ids(&cache.workspaces(), |w| &w.id), ["w1", "w2", "w3"]);
    }

    // --- reseeding ---------------------------------------------------------

    #[test]
    fn a_seed_inserts_its_rows() {
        let mut cache = EntityCache::new();
        let generation = cache.begin_reseed();
        assert!(cache.apply_seed(
            &Scope::Projects,
            generation,
            Seed::Projects(vec![project_dto("p1"), project_dto("p2")]),
        ));
        assert_eq!(ids(&cache.projects(), |p| &p.id), ["p1", "p2"]);
    }

    /// A row deleted during an outage is never mentioned again, so a reseed
    /// has to prune rather than merely upsert.
    #[test]
    fn a_seed_prunes_rows_it_did_not_mention() {
        let mut cache = EntityCache::new();
        cache.apply_workspace(ws_in("w1", "p1", "r1"));
        cache.apply_workspace(ws_in("w-gone", "p1", "r1"));

        let scope = Scope::Workspaces {
            project_id: "p1".into(),
            repo_id: "r1".into(),
        };
        let generation = cache.begin_reseed();
        cache.apply_seed(
            &scope,
            generation,
            Seed::Workspaces(vec![ws_in("w1", "p1", "r1")]),
        );

        assert_eq!(ids(&cache.workspaces(), |w| &w.id), ["w1"]);
    }

    /// **The sibling-wipe bug.** One store, two streams. A seed that is not
    /// scoped deletes every row the other stream owns — in the React app,
    /// opening one workspace emptied the sidebar of all the others.
    #[test]
    fn a_seed_does_not_prune_another_repos_rows() {
        let mut cache = EntityCache::new();
        cache.apply_workspace(ws_in("mine", "p1", "r1"));
        cache.apply_workspace(ws_in("theirs", "p1", "r2"));

        let scope = Scope::Workspaces {
            project_id: "p1".into(),
            repo_id: "r1".into(),
        };
        let generation = cache.begin_reseed();
        cache.apply_seed(&scope, generation, Seed::Workspaces(vec![]));

        assert_eq!(
            ids(&cache.workspaces(), |w| &w.id),
            ["theirs"],
            "r1's empty seed is authoritative over r1 and nothing else"
        );
    }

    #[test]
    fn a_repo_seed_does_not_prune_another_projects_repos() {
        let mut cache = EntityCache::new();
        cache.apply_repo(repo_dto("r1", "p1"));
        cache.apply_repo(repo_dto("r2", "p2"));

        let scope = Scope::Repos {
            project_id: "p1".into(),
        };
        let generation = cache.begin_reseed();
        cache.apply_seed(&scope, generation, Seed::Repos(vec![]));

        assert_eq!(ids(&cache.repos(), |r| &r.id), ["r2"]);
    }

    /// A reconnect during a slow seed starts a newer one. The older answer is
    /// stale by the time it lands and must lose, or the cache settles on it.
    #[test]
    fn a_superseded_seed_is_discarded() {
        let mut cache = EntityCache::new();
        let stale = cache.begin_reseed();
        let fresh = cache.begin_reseed();

        assert!(cache.is_current(fresh));
        assert!(!cache.is_current(stale));

        assert!(!cache.apply_seed(
            &Scope::Projects,
            stale,
            Seed::Projects(vec![project_dto("from-the-old-seed")]),
        ));
        assert!(cache.projects().is_empty(), "the cache is untouched");

        assert!(cache.apply_seed(
            &Scope::Projects,
            fresh,
            Seed::Projects(vec![project_dto("p1")]),
        ));
        assert_eq!(ids(&cache.projects(), |p| &p.id), ["p1"]);
    }

    /// A seed of the wrong kind is a call-site error, and dropping it beats
    /// aborting the app over an arm that should be unreachable.
    #[test]
    fn a_seed_of_the_wrong_kind_is_dropped() {
        let mut cache = EntityCache::new();
        let generation = cache.begin_reseed();
        assert!(!cache.apply_seed(
            &Scope::Projects,
            generation,
            Seed::Repos(vec![repo_dto("r1", "p1")]),
        ));
        assert!(cache.repos().is_empty());
    }

    /// Switching projects must not leave the old project's repos on screen
    /// while the new seed is in flight.
    #[test]
    fn forgetting_a_project_drops_its_repos_and_workspaces() {
        let mut cache = EntityCache::new();
        cache.apply_repo(repo_dto("r1", "p1"));
        cache.apply_repo(repo_dto("r2", "p2"));
        cache.apply_workspace(ws_in("w1", "p1", "r1"));
        cache.apply_workspace(ws_in("w2", "p2", "r2"));

        cache.forget_project("p1");

        assert_eq!(ids(&cache.repos(), |r| &r.id), ["r2"]);
        assert_eq!(ids(&cache.workspaces(), |w| &w.id), ["w2"]);
    }

    #[test]
    fn forgetting_a_project_leaves_the_project_list_alone() {
        let mut cache = EntityCache::new();
        cache.apply_project(project_dto("p1"));
        cache.forget_project("p1");
        assert_eq!(
            ids(&cache.projects(), |p| &p.id),
            ["p1"],
            "the project is still a project; only its scoped children are dropped"
        );
    }
}
