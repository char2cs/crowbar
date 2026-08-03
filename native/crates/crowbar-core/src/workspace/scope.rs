//! The hierarchical scope (owning project + repo) of each workspace.
//!
//! Ported from `web/src/lib/workspace-scope.ts`. Every files/git/lsp/terminal
//! route nests under the owning project+repo, so every scoped API call has to
//! resolve a workspace id to its `{projectId, repoId}` first — that resolution
//! is what this module owns. See `crate::workspace` module docs for why this
//! is an owned [`WorkspaceScopeRegistry`] here rather than the TS file's
//! module-level global.

use std::collections::HashMap;

/// The hierarchical scope of one workspace: which project and repo own it.
///
/// A **home** (project-level) workspace has no owning repo; its `repo_id` is
/// recorded as `""` rather than modelled as an `Option`, matching the TS
/// `WorkspaceScope` interface exactly — [`crate::workspace::scope_url`] is the
/// module that gives that empty string meaning (`workspaceBase`'s `/home`
/// route, `isHomeWorkspace`).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WorkspaceScope {
    pub project_id: String,
    pub repo_id: String,
    pub ws_id: String,
}

/// Registry mapping `wsId -> WorkspaceScope`, plus which workspace is
/// currently active. Framework-free: no gpui, no reactive wiring — the plain
/// state `crowbar-state` will eventually wrap in an `Entity<T>`.
#[derive(Debug, Clone, Default, PartialEq, Eq)]
pub struct WorkspaceScopeRegistry {
    active_workspace_id: Option<String>,
    scopes: HashMap<String, WorkspaceScope>,
}

impl WorkspaceScopeRegistry {
    /// An empty registry with no active workspace.
    #[must_use]
    pub fn new() -> Self {
        Self::default()
    }

    /// Set the wsId of the active workspace route, without touching any
    /// recorded scope. Mirrors `setActiveScopeWorkspaceId`.
    pub fn set_active_scope_workspace_id(&mut self, ws_id: Option<String>) {
        self.active_workspace_id = ws_id;
    }

    /// Record `scope` and make it the active workspace. Mirrors
    /// `setWorkspaceScope`.
    pub fn set_workspace_scope(&mut self, scope: WorkspaceScope) {
        self.active_workspace_id = Some(scope.ws_id.clone());
        self.scopes.insert(scope.ws_id.clone(), scope);
    }

    /// Record `scope` **without** making it the active workspace. Mirrors
    /// `recordWorkspaceScope` — the sidebar store calls this for every
    /// workspace as its data arrives, so an action on a workspace the user
    /// never navigated to (e.g. Retry/Detach… on a placeholder row) can still
    /// build a scoped URL; without this, `workspace_base` throws/returns an
    /// error for that row and the action silently no-ops.
    pub fn record_workspace_scope(&mut self, scope: WorkspaceScope) {
        self.scopes.insert(scope.ws_id.clone(), scope);
    }

    /// Parse an `/ide/:projectId/:repoId/:wsId` pathname and record its scope,
    /// then return it (`None` for a non-`/ide` path). Mirrors
    /// `recordWorkspaceScopeFromPath`.
    pub fn record_workspace_scope_from_path(&mut self, pathname: &str) -> Option<WorkspaceScope> {
        let scope = parse_workspace_scope_from_path(pathname)?;
        self.set_workspace_scope(scope.clone());
        Some(scope)
    }

    /// Resolve the hierarchical scope for `ws_id` (falls back to the active
    /// workspace when `None`). Mirrors `getWorkspaceScope`.
    ///
    /// An id that resolves to `""` — either an explicitly-passed empty string,
    /// or an active-workspace id that is itself `""` — returns `None` rather
    /// than looking `""` up in the map; see the module-level doc for why that
    /// matches the TS `??` + truthy-check pair exactly.
    #[must_use]
    pub fn get_workspace_scope(&self, ws_id: Option<&str>) -> Option<&WorkspaceScope> {
        let id = ws_id.or(self.active_workspace_id.as_deref())?;
        if id.is_empty() {
            return None;
        }
        self.scopes.get(id)
    }
}

/// Pure parse of an `/ide/:projectId/:repoId/:wsId` pathname into its scope,
/// **without** recording it (no side effect) — `None` for a non-`/ide` path.
/// Mirrors `parseWorkspaceScopeFromPath`.
///
/// Matches the TS `IDE_ROUTE` regex (`/\/ide\/([^/]+)\/([^/]+)\/([^/]+)/`)
/// byte for byte: not anchored to the start of the string (so it also matches
/// a hash-history in-hash path), each segment stops at the next `/`, and — the
/// part a naive `split('/')` would get wrong — if the first `/ide/` in the
/// string cannot be followed by three non-empty segments, the search continues
/// at the next `/ide/` rather than failing outright, the same way a regex
/// engine backtracks across every possible match start. The app never
/// constructs a pathname with more than one `/ide/`, so this only matters for
/// exactness, not for anything actually reachable today.
#[must_use]
pub fn parse_workspace_scope_from_path(pathname: &str) -> Option<WorkspaceScope> {
    const NEEDLE: &str = "/ide/";
    let mut search_from = 0usize;
    while let Some(offset) = pathname[search_from..].find(NEEDLE) {
        let start = search_from + offset;
        let after = &pathname[start + NEEDLE.len()..];
        if let Some(scope) = parse_three_segments(after) {
            return Some(scope);
        }
        // `/ide/`'s own leading `/` is one ASCII byte, so `start + 1` is
        // always a valid char boundary to resume the search from.
        search_from = start + 1;
    }
    None
}

fn parse_three_segments(after: &str) -> Option<WorkspaceScope> {
    let mut segments = after.split('/');
    // `str::split`'s first `.next()` is `Some` for every input, including
    // `""` (`"".split('/').next() == Some("")`), so this particular `?` can
    // never actually take its early-return arm — confirmed rather than
    // assumed. It stays for symmetry with the two calls below, which *can*
    // (`after` with fewer than two `/`s exhausts the iterator on the second
    // or third call — see `rejects_an_ide_path_with_too_few_segments` and
    // `rejects_an_ide_path_with_exactly_two_segments` below).
    let project_id = segments.next()?;
    let repo_id = segments.next()?;
    let ws_id = segments.next()?;
    if project_id.is_empty() || repo_id.is_empty() || ws_id.is_empty() {
        return None;
    }
    Some(WorkspaceScope {
        project_id: project_id.to_string(),
        repo_id: repo_id.to_string(),
        ws_id: ws_id.to_string(),
    })
}

#[cfg(test)]
mod tests {
    use super::{WorkspaceScope, WorkspaceScopeRegistry, parse_workspace_scope_from_path};
    use crate::workspace::scope_url::workspace_base;

    fn scope(project_id: &str, repo_id: &str, ws_id: &str) -> WorkspaceScope {
        WorkspaceScope {
            project_id: project_id.to_string(),
            repo_id: repo_id.to_string(),
            ws_id: ws_id.to_string(),
        }
    }

    // --- ported from web/src/__tests__/lib/workspace-scope.test.ts ---
    // (`describe('recordWorkspaceScopeFromPath', …)` + `describe('parseWorkspaceScopeFromPath', …)`)
    //
    // The original file's own comment: a regression for the §14 add-repo
    // failure, where the scope was recorded only in the route component's
    // post-render `useEffect`, but `WorkspaceView` builds scoped URLs (via
    // `workspaceBase`) DURING the same render — before that effect ran. The
    // first render threw and tripped the ErrorBoundary permanently. These
    // tests exist to keep recording synchronous.

    #[test]
    fn records_scope_so_workspace_base_resolves_synchronously() {
        let mut registry = WorkspaceScopeRegistry::new();
        let recorded = registry.record_workspace_scope_from_path("/ide/proj-1/repo-2/ws-3");
        assert_eq!(recorded, Some(scope("proj-1", "repo-2", "ws-3")));
        // Resolvable immediately — no route effect needed.
        assert_eq!(
            registry.get_workspace_scope(Some("ws-3")),
            Some(&scope("proj-1", "repo-2", "ws-3"))
        );
        assert_eq!(
            workspace_base(&registry, "ws-3").as_deref(),
            Ok("/v0/projects/proj-1/repos/repo-2/workspaces/ws-3")
        );
    }

    #[test]
    fn captures_exactly_three_ide_segments_ignoring_deeper_path_parts() {
        let mut registry = WorkspaceScopeRegistry::new();
        let recorded = registry.record_workspace_scope_from_path("/ide/p/r/w/some/deeper/route");
        assert_eq!(recorded, Some(scope("p", "r", "w")));
        assert_eq!(
            workspace_base(&registry, "w").as_deref(),
            Ok("/v0/projects/p/repos/r/workspaces/w")
        );
    }

    #[test]
    fn recording_makes_the_workspace_active_with_no_arg_lookup() {
        let mut registry = WorkspaceScopeRegistry::new();
        registry.record_workspace_scope_from_path("/ide/p9/r9/w9");
        assert_eq!(
            registry.get_workspace_scope(None),
            Some(&scope("p9", "r9", "w9"))
        );
    }

    #[test]
    fn returns_none_and_records_nothing_for_a_non_ide_path() {
        let mut registry = WorkspaceScopeRegistry::new();
        assert_eq!(registry.record_workspace_scope_from_path("/"), None);
        assert_eq!(registry.record_workspace_scope_from_path("/chat/abc"), None);
        assert_eq!(registry.get_workspace_scope(Some("w9")), None);
    }

    #[test]
    fn parses_the_scope_from_an_ide_path() {
        assert_eq!(
            parse_workspace_scope_from_path("/ide/p1/r1/ws1"),
            Some(scope("p1", "r1", "ws1"))
        );
    }

    #[test]
    fn returns_none_for_a_legacy_workspaces_path_and_other_non_ide_paths() {
        assert_eq!(parse_workspace_scope_from_path("/workspaces/ws1"), None);
        assert_eq!(parse_workspace_scope_from_path("/"), None);
        assert_eq!(parse_workspace_scope_from_path("/chat/abc"), None);
    }

    // The TS suite's fourth `parseWorkspaceScopeFromPath` case — "does NOT
    // record into the registry (pure read, unlike recordWorkspaceScopeFromPath)"
    // — is dropped rather than ported. It guarded against the pure parser
    // reaching into the shared global `_scopes` map; here
    // `parse_workspace_scope_from_path` takes no `&mut WorkspaceScopeRegistry`
    // at all, so the property is enforced by the function's signature, not by
    // its body. A test asserting it would pass unconditionally — it cannot be
    // made to fail by any one-line change to the function, which is exactly
    // the "declaration, not behaviour" shape this port's brief asked to be
    // caught rather than shipped.

    // --- new: registry API the TS test file didn't exercise directly ---

    #[test]
    fn record_workspace_scope_does_not_activate_it() {
        // Mirrors `recordWorkspaceScope`'s own doc comment: the sidebar store
        // calls this for every workspace, not just the one the user is on, and
        // it must NOT steal "active" from whatever the route already set.
        let mut registry = WorkspaceScopeRegistry::new();
        registry.set_workspace_scope(scope("p1", "r1", "active-ws"));
        registry.record_workspace_scope(scope("p1", "r1", "other-ws"));
        assert_eq!(
            registry.get_workspace_scope(None),
            Some(&scope("p1", "r1", "active-ws"))
        );
        // But the recorded (non-active) scope is still resolvable by id.
        assert_eq!(
            registry.get_workspace_scope(Some("other-ws")),
            Some(&scope("p1", "r1", "other-ws"))
        );
    }

    #[test]
    fn set_active_scope_workspace_id_switches_the_default_lookup() {
        let mut registry = WorkspaceScopeRegistry::new();
        registry.record_workspace_scope(scope("p1", "r1", "a"));
        registry.record_workspace_scope(scope("p1", "r1", "b"));
        registry.set_active_scope_workspace_id(Some("b".to_string()));
        assert_eq!(
            registry.get_workspace_scope(None),
            Some(&scope("p1", "r1", "b"))
        );
        registry.set_active_scope_workspace_id(None);
        assert_eq!(registry.get_workspace_scope(None), None);
    }

    #[test]
    fn unknown_workspace_id_resolves_to_none() {
        let registry = WorkspaceScopeRegistry::new();
        assert_eq!(registry.get_workspace_scope(Some("ghost")), None);
        assert_eq!(registry.get_workspace_scope(None), None);
    }

    #[test]
    fn an_explicit_empty_string_id_never_resolves_even_if_active_is_set() {
        // See the module doc: `wsId ?? _activeWorkspaceId` does not treat ""
        // as nullish, but the following `if (!id) return null` does — so an
        // explicitly empty id is always "no scope", regardless of what's
        // active. Passing None (no `wsId` argument) still falls through to
        // the active id, and is unaffected by this rule.
        let mut registry = WorkspaceScopeRegistry::new();
        registry.set_workspace_scope(scope("p1", "r1", "active-ws"));
        assert_eq!(registry.get_workspace_scope(Some("")), None);
        assert_eq!(
            registry.get_workspace_scope(None),
            Some(&scope("p1", "r1", "active-ws"))
        );
    }

    #[test]
    fn overwriting_a_scope_replaces_it_by_ws_id() {
        let mut registry = WorkspaceScopeRegistry::new();
        registry.record_workspace_scope(scope("p1", "r1", "w"));
        registry.record_workspace_scope(scope("p2", "r2", "w"));
        assert_eq!(
            registry.get_workspace_scope(Some("w")),
            Some(&scope("p2", "r2", "w"))
        );
    }

    // Exercises the "first /ide/ occurrence doesn't parse, try the next one"
    // backtracking branch: the first "/ide/" is immediately followed by an
    // empty segment ("//ide/..."), so it can't yield three non-empty
    // segments, and the parser must fall through to the second "/ide/".
    #[test]
    fn falls_back_to_a_later_ide_occurrence_when_the_first_does_not_parse() {
        assert_eq!(
            parse_workspace_scope_from_path("/ide/p//ide/x/y/z"),
            Some(scope("x", "y", "z"))
        );
    }

    #[test]
    fn rejects_an_ide_path_with_an_empty_middle_segment() {
        assert_eq!(parse_workspace_scope_from_path("/ide/p//w"), None);
    }

    #[test]
    fn rejects_an_ide_path_with_too_few_segments() {
        // Only one segment after "/ide/" — the repoId capture group has
        // nothing to match, same as the regex failing to find a third "/".
        assert_eq!(parse_workspace_scope_from_path("/ide/p"), None);
    }

    #[test]
    fn rejects_an_ide_path_with_exactly_two_segments() {
        // projectId and repoId are present; wsId has nothing to match.
        assert_eq!(parse_workspace_scope_from_path("/ide/p/r"), None);
    }

    #[test]
    fn registry_default_and_new_are_both_empty() {
        assert_eq!(
            WorkspaceScopeRegistry::default(),
            WorkspaceScopeRegistry::new()
        );
        assert_eq!(
            WorkspaceScopeRegistry::new().get_workspace_scope(None),
            None
        );
    }
}
