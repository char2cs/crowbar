//! The branch import dialog's data: a repository's remote branches and open pull requests, and the
//! batch import call.

use std::sync::{Arc, Mutex};

use daemonapi::{Call, DaemonClient, daemon_read::read};
use serde_json::json;
use tokio::runtime::Handle;

use crate::branch_entry::BranchEntry;
use crate::branch_listing::BranchListing;
use crate::change_signal::ChangeSignal;
use crate::error::Error;
use crate::held::Held;
use crate::hub::Hub;
use crate::import_plan::{self, ImportPlan};
use crate::import_scope::ImportScope;
use crate::loadable::Loadable;
use crate::outcome::OutcomeSink;
use crate::plan::Plan;
use crate::pr_link::PrLink;
use crate::state::State;
use crate::target::resolve;
use crate::{branch_rows, runner};
use daemonapi::{new_id, routes};

/// The branches of one repository's remote, loaded once and shown by the import dialog.
pub struct BranchImport {
    scope: ImportScope,
    client: DaemonClient,
    hub: Arc<Hub>,
    handle: Handle,
    outcome: OutcomeSink,
    signal: ChangeSignal,
    held: Mutex<Held>,
}

impl std::fmt::Debug for BranchImport {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("BranchImport")
            .field("scope", &self.scope)
            .finish_non_exhaustive()
    }
}

fn scope_of(
    state: &State,
    row: &str,
) -> Option<(ImportScope, DaemonClient)> {
    let target = resolve(state, row).filter(|target| !target.repo.is_empty())?;
    let slot = state
        .hosts
        .iter()
        .find(|slot| slot.store.projects.get(&target.project).is_some())?;
    let default_branch = slot
        .store
        .repos
        .get(&target.repo)
        .map_or_default(|repo| repo.default_branch.clone());
    let scope = ImportScope {
        host: slot.spec.id.clone(),
        project: target.project,
        repo: target.repo,
        default_branch,
    };
    Some((scope, slot.client.clone()))
}

impl BranchImport {
    pub(crate) fn open(
        parts: (&Arc<Hub>, &Handle, &OutcomeSink),
        row: &str,
        signal: ChangeSignal,
    ) -> Result<Arc<Self>, Error> {
        let (hub, handle, outcome) = parts;
        let (scope, client) = hub
            .read(|state| scope_of(state, row))
            .ok_or_else(|| Error::Stream("this row is not a repository".to_owned()))?;
        let import = Arc::new(Self {
            scope,
            client,
            hub: Arc::clone(hub),
            handle: handle.clone(),
            outcome: Arc::clone(outcome),
            signal,
            held: Mutex::new(Held::loading()),
        });
        import.load();
        Ok(import)
    }

    fn base(&self) -> String {
        routes::scope(&self.scope.project, &self.scope.repo)
    }

    fn load(self: &Arc<Self>) {
        let (branches, links) = (Arc::clone(self), Arc::clone(self));
        self.handle
            .spawn(async move { branches.load_branches().await });
        self.handle.spawn(async move { links.load_links().await });
    }

    fn hold(
        &self,
        change: impl FnOnce(&mut Held),
    ) {
        if let Ok(mut held) = self.held.lock() {
            change(&mut held);
        }
        (self.signal)();
    }

    async fn load_branches(&self) {
        let path = format!("{}/branches", self.base());
        let read = read::<Vec<BranchEntry>>(&self.client, path).await;
        let loaded = read
            .inspect_err(|error| tracing::warn!(%error, "the remote branches could not be listed"))
            .map_or(Loadable::Failed, Loadable::Ready);
        self.hold(|held| held.branches = loaded);
    }

    async fn load_links(&self) {
        let path = format!("{}/pull-requests", self.base());
        let links = read::<Vec<PrLink>>(&self.client, path)
            .await
            .inspect_err(|error| tracing::warn!(%error, "the open pull requests could not be read"))
            .unwrap_or_default();
        self.hold(|held| held.links = links);
    }

    /// The branches as far as the daemon has answered; protected ones first.
    pub fn listing(&self) -> BranchListing {
        let Ok(held) = self.held.lock() else {
            return BranchListing::Failed;
        };
        match &held.branches {
            Loadable::Ready(list) => BranchListing::Ready(branch_rows::of(list)),
            Loadable::Failed => BranchListing::Failed,
            Loadable::Loading => BranchListing::Loading,
        }
    }

    /// What importing `selected` would do.
    pub fn plan(
        &self,
        selected: &[String],
    ) -> ImportPlan {
        let Ok(held) = self.held.lock() else {
            return ImportPlan::default();
        };
        let branches = match &held.branches {
            Loadable::Ready(list) => list.as_slice(),
            _ => &[],
        };
        import_plan::of(
            selected,
            &held.links,
            (branches, &self.scope.default_branch),
        )
    }

    /// Imports `branches` as one batch, each under a chat id minted here; those in `locked` come
    /// in locked.
    pub fn import(
        &self,
        branches: &[String],
        locked: &[String],
    ) {
        let named: Vec<_> = branches
            .iter()
            .map(|name| {
                json!({"name": name, "chatId": new_id::uuid(), "lock": locked.contains(name)})
            })
            .collect();
        let call = Call::post(format!("{}/chats/import-batch", self.base()))
            .json(json!({"branches": named}))
            .idempotent(&new_id::uuid());
        let plan = Plan::of(&self.scope.host, vec![call]);
        let (hub, outcome) = (Arc::clone(&self.hub), Arc::clone(&self.outcome));
        self.handle
            .spawn(async move { runner::run(&hub, plan, &outcome).await });
    }
}

#[cfg(test)]
mod tests {
    use tokio::sync::Notify;

    use super::*;
    use crate::availability::Availability;
    use crate::hub::Hub;
    use crate::import_branch::ImportBranch;
    use crate::test_support::{FakeDaemon, state};

    const BRANCHES: &str = r#"[{"name":"free"},{"name":"main","isProtected":true},{"name":"done","hasWorkspace":true}]"#;
    const LINKS: &str = r#"[{"head":"free","base":"parent"},{"head":"parent","base":"main"}]"#;
    const BATCH: &str = "/v0/projects/p/repos/r/chats/import-batch";

    fn hush() {}

    fn deaf(_: crate::outcome::Outcome) {}

    fn rig() -> (Arc<Hub>, Arc<FakeDaemon>, OutcomeSink) {
        let (state, daemon) = state();
        let hub = Hub::new(state, Arc::new(hush));
        (hub, daemon, Arc::new(deaf))
    }

    async fn until_ready(
        import: &BranchImport,
        changed: &Notify,
    ) -> Vec<ImportBranch> {
        while import.listing() == BranchListing::Loading {
            changed.notified().await;
        }
        match import.listing() {
            BranchListing::Ready(rows) => rows,
            _ => Vec::new(),
        }
    }

    async fn until_parents(
        import: &BranchImport,
        changed: &Notify,
    ) {
        while import.plan(&["free".to_owned()]).parent_count == 0 {
            changed.notified().await;
        }
    }

    async fn until_posted(daemon: &FakeDaemon) -> serde_json::Value {
        let mut body = None;
        while body.is_none() {
            let posted = daemon.seen().into_iter().find(|call| call.path == BATCH);
            body = posted.and_then(|call| call.body);
            tokio::task::yield_now().await;
        }
        body.unwrap_or_default()
    }

    #[tokio::test]
    async fn the_remote_branches_list_protected_first_and_an_import_posts_one_batch() {
        let (hub, daemon, outcome) = rig();
        daemon.ok("GET", "/v0/projects/p/repos/r/branches", BRANCHES);
        daemon.ok("GET", "/v0/projects/p/repos/r/pull-requests", LINKS);
        daemon.ok("POST", BATCH, r#"{"id":"op"}"#);
        let changed = Arc::new(Notify::new());
        let wake = Arc::clone(&changed);
        let signal: ChangeSignal = Arc::new(move || wake.notify_one());
        let parts = (&hub, &Handle::current(), &outcome);
        let import = BranchImport::open(parts, "rdo", signal).unwrap();
        assert_eq!(import.listing(), BranchListing::Loading);
        let rows = until_ready(&import, &changed).await;
        let seen: Vec<_> = rows
            .iter()
            .map(|row| (row.name.as_str(), row.availability))
            .collect();
        assert_eq!(
            seen,
            [
                ("main", Availability::Protected),
                ("free", Availability::Importable),
                ("done", Availability::Imported)
            ]
        );
        until_parents(&import, &changed).await;
        import.import(&["free".to_owned()], &["free".to_owned()]);
        let body = until_posted(&daemon).await;
        assert_eq!(body["branches"][0]["name"], "free");
        assert_eq!(body["branches"][0]["lock"], true);
    }

    #[tokio::test]
    async fn a_row_that_is_no_repository_cannot_be_imported_into() {
        let (hub, _, outcome) = rig();
        let parts = (&hub, &Handle::current(), &outcome);
        assert!(BranchImport::open(parts, "hc", Arc::new(hush)).is_err());
    }
}
