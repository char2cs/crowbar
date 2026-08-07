//! Which streams should be open, given what the store currently knows.
//!
//! The §7 startup sequence of `web/src/components/app-sync-provider.tsx`,
//! reduced to the decision it actually makes. React expresses it as an effect
//! with a subscription array rebuilt on every change; here it is a pure
//! function from store state to the set of scopes that should be open, plus
//! one call that makes the world match.
//!
//! # The order is the behaviour
//!
//! On a project switch the previous project's streams are closed **before**
//! the new ones open, and the previous project's cached rows go with them, so
//! the old project's repos leave the screen at once rather than lingering
//! until the new seed lands. [`reconcile`] closes first for that reason and
//! not merely for tidiness.

use crowbar_core::sidebar::cache::Scope;
use crowbar_state::{DaemonSync, SidebarStore};
use crowbar_ui::gpui::{App, Entity};

/// The scopes that should be open for `store`'s current state.
///
/// Always includes the project list: it is what tells the app a project
/// exists at all, and closing it would strand the switcher.
#[must_use]
pub fn wanted(store: &SidebarStore) -> Vec<Scope> {
    let mut scopes = vec![Scope::Projects];
    let Some(project_id) = store.active_project_id() else {
        return scopes;
    };
    scopes.push(Scope::Repos {
        project_id: project_id.to_owned(),
    });
    for repo in store.repos() {
        scopes.push(Scope::Workspaces {
            project_id: project_id.to_owned(),
            repo_id: repo.id.clone(),
        });
    }
    scopes
}

/// Adopt a project when none is active yet.
///
/// The React app takes the active project from the route; this binary has no
/// router, so on a cold start it opens the first project the daemon reports.
/// Returns whether anything changed, so a caller can avoid a redundant
/// reconcile.
pub fn adopt_first_project(store: &Entity<SidebarStore>, cx: &mut App) -> bool {
    store.update(cx, |store, cx| {
        if store.active_project_id().is_some() {
            return false;
        }
        let Some(first) = store.projects().first().map(|project| project.id.clone()) else {
            return false;
        };
        store.set_active_project(Some(first), cx);
        true
    })
}

/// Make the open streams match [`wanted`].
pub fn reconcile(store: &Entity<SidebarStore>, sync: &mut DaemonSync, cx: &mut App) {
    let scopes = store.read(cx).pipe(wanted);
    let keep: Vec<String> = scopes.iter().map(Scope::path).collect();
    // Closed first — see the module docs.
    sync.retain(&keep, cx);
    for scope in scopes {
        sync.open(scope, cx);
    }
}

/// `x.pipe(f)` reads better than `f(&x)` at the one call site that needs it,
/// where `x` is a borrow that cannot outlive the statement.
trait Pipe {
    fn pipe<R>(&self, f: impl FnOnce(&Self) -> R) -> R {
        f(self)
    }
}
impl Pipe for SidebarStore {}
