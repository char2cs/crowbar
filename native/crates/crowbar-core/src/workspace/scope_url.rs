//! Workspace-scoped REST base-path construction.
//!
//! Ported from `web/src/lib/workspace-scope-url.ts`. Every
//! files/git/lsp/terminal route nests under the owning project+repo now;
//! callers pass only the `wsId` they hold, and the project/repo segments are
//! resolved from a [`WorkspaceScopeRegistry`].
//!
//! **`workspace-scope-url.ts` has zero dedicated tests upstream** — no
//! `workspace-scope-url.test.ts` exists anywhere under `web/src/__tests__`,
//! despite `workspaceBase` running on every scoped API call in the app (flagged
//! by `native/mapping/tier-a-denominator.md` §"Workspace scoping"). Two of the
//! cases below (`records_scope_so_workspace_base_resolves_synchronously` and
//! `captures_exactly_three_ide_segments_ignoring_deeper_path_parts`, both in
//! `super::scope`'s test module) are ported — they exercise `workspace_base`
//! as part of a `workspace-scope.test.ts` regression that happens to call
//! through it. Every test in *this* module's own `#[cfg(test)]` block is new:
//! written for this port, not translated from an existing case.

use super::scope::WorkspaceScopeRegistry;

/// [`workspace_base`] was asked for a workspace whose scope was never
/// recorded. Mirrors the TS `throw new Error(...)` — surfaced as a typed error
/// rather than a panic so a stale/mis-ordered call fails loudly at the call
/// site instead of building a URL with a missing path segment (or, in Rust,
/// aborting the process).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct UnknownWorkspaceScope {
    pub ws_id: String,
}

impl std::fmt::Display for UnknownWorkspaceScope {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        write!(
            f,
            "no project/repo scope recorded for workspace {}",
            self.ws_id
        )
    }
}

impl std::error::Error for UnknownWorkspaceScope {}

/// Build the hierarchical base for a workspace-scoped API/WS URL. Mirrors
/// `workspaceBase`.
///
/// # Errors
///
/// Returns [`UnknownWorkspaceScope`] when `ws_id` has no scope recorded in
/// `registry` — the Rust equivalent of the TS function's `throw`.
pub fn workspace_base(
    registry: &WorkspaceScopeRegistry,
    ws_id: &str,
) -> Result<String, UnknownWorkspaceScope> {
    let scope = registry
        .get_workspace_scope(Some(ws_id))
        .ok_or_else(|| UnknownWorkspaceScope {
            ws_id: ws_id.to_string(),
        })?;
    let project = encode_uri_component(&scope.project_id);
    // Home workspaces have no repoId — they route to /v0/projects/:p/home/...
    if scope.repo_id.is_empty() {
        return Ok(format!("/v0/projects/{project}/home"));
    }
    let repo = encode_uri_component(&scope.repo_id);
    let ws = encode_uri_component(ws_id);
    Ok(format!(
        "/v0/projects/{project}/repos/{repo}/workspaces/{ws}"
    ))
}

/// A home (project-level) workspace has no owning repo: its scope is recorded
/// with an empty `repo_id`, which is how [`workspace_base`] routes it to
/// `/home/…`. The home workspace deliberately has no git surface on the
/// backend, so callers use this to skip git data/streams (files and threads
/// ARE home capabilities and stay enabled). Returns `false` when the scope is
/// unknown, so non-home workspaces keep their full surface. Mirrors
/// `isHomeWorkspace`.
#[must_use]
pub fn is_home_workspace(registry: &WorkspaceScopeRegistry, ws_id: &str) -> bool {
    registry
        .get_workspace_scope(Some(ws_id))
        .is_some_and(|scope| scope.repo_id.is_empty())
}

/// Percent-encode a path segment the way JavaScript's `encodeURIComponent`
/// does: every byte outside the unreserved set (`A-Z a-z 0-9 - _ . ! ~ * ' (
/// )`) becomes an uppercase-hex `%XX` escape. `workspaceBase` calls
/// `encodeURIComponent` on every id it interpolates into the path.
///
/// Hand-rolled rather than pulling in a URL-encoding crate: `crowbar-core`'s
/// dependency list is `crowbar-proto` and `crowbar-client` only (§4.2), the
/// unreserved set is a fixed, 32-character table, and encoding is a single
/// pass with no parsing — a dependency would not make this simpler, only add
/// one. Operating on UTF-8 bytes rather than `char`s is intentional and
/// correct: every unreserved character is single-byte ASCII, so a multi-byte
/// UTF-8 character's continuation bytes (always `>= 0x80`) can never collide
/// with the unreserved set, and encoding byte-by-byte reproduces
/// `encodeURIComponent`'s own per-UTF-8-byte escaping exactly.
fn encode_uri_component(input: &str) -> String {
    const HEX_DIGITS: &[u8; 16] = b"0123456789ABCDEF";

    let mut out = String::with_capacity(input.len());
    for byte in input.bytes() {
        match byte {
            b'A'..=b'Z'
            | b'a'..=b'z'
            | b'0'..=b'9'
            | b'-'
            | b'_'
            | b'.'
            | b'!'
            | b'~'
            | b'*'
            | b'\''
            | b'('
            | b')' => out.push(byte as char),
            _ => {
                out.push('%');
                out.push(HEX_DIGITS[(byte >> 4) as usize] as char);
                out.push(HEX_DIGITS[(byte & 0x0F) as usize] as char);
            }
        }
    }
    out
}

#[cfg(test)]
mod tests {
    use super::{encode_uri_component, is_home_workspace, workspace_base};
    use crate::workspace::scope::{WorkspaceScope, WorkspaceScopeRegistry};

    fn registry_with(scopes: &[(&str, &str, &str)]) -> WorkspaceScopeRegistry {
        let mut registry = WorkspaceScopeRegistry::new();
        for &(project_id, repo_id, ws_id) in scopes {
            registry.record_workspace_scope(WorkspaceScope {
                project_id: project_id.to_string(),
                repo_id: repo_id.to_string(),
                ws_id: ws_id.to_string(),
            });
        }
        registry
    }

    #[test]
    fn builds_the_project_repo_workspace_path() {
        let registry = registry_with(&[("proj-1", "repo-2", "ws-3")]);
        assert_eq!(
            workspace_base(&registry, "ws-3"),
            Ok("/v0/projects/proj-1/repos/repo-2/workspaces/ws-3".to_string())
        );
    }

    #[test]
    fn routes_a_home_workspace_to_the_home_path_with_no_repo_segment() {
        let registry = registry_with(&[("proj-1", "", "home-ws")]);
        assert_eq!(
            workspace_base(&registry, "home-ws"),
            Ok("/v0/projects/proj-1/home".to_string())
        );
    }

    #[test]
    fn errors_for_a_workspace_with_no_recorded_scope() {
        let registry = WorkspaceScopeRegistry::new();
        let err = workspace_base(&registry, "ghost").unwrap_err();
        assert_eq!(err.ws_id, "ghost");
        assert_eq!(
            err.to_string(),
            "no project/repo scope recorded for workspace ghost"
        );
    }

    #[test]
    fn percent_encodes_every_interpolated_segment() {
        // A project/repo/workspace id containing characters that are not in
        // the unreserved set must come out escaped, in every position.
        let registry = registry_with(&[("proj one/two", "repo?three", "ws four#five")]);
        assert_eq!(
            workspace_base(&registry, "ws four#five"),
            Ok(
                "/v0/projects/proj%20one%2Ftwo/repos/repo%3Fthree/workspaces/ws%20four%23five"
                    .to_string()
            )
        );
    }

    #[test]
    fn percent_encoding_preserves_the_full_unreserved_set() {
        assert_eq!(encode_uri_component("Azaz09-_.!~*'()"), "Azaz09-_.!~*'()");
    }

    #[test]
    fn percent_encoding_escapes_a_multi_byte_utf8_character_byte_by_byte() {
        // "é" is the two UTF-8 bytes 0xC3 0xA9 — encodeURIComponent("é") is
        // "%C3%A9", not a single escape.
        assert_eq!(encode_uri_component("é"), "%C3%A9");
    }

    #[test]
    fn is_home_workspace_true_only_for_an_empty_repo_id() {
        let registry = registry_with(&[("p", "", "home-ws"), ("p", "r", "git-ws")]);
        assert!(is_home_workspace(&registry, "home-ws"));
        assert!(!is_home_workspace(&registry, "git-ws"));
    }

    #[test]
    fn is_home_workspace_false_for_an_unknown_workspace() {
        let registry = WorkspaceScopeRegistry::new();
        assert!(!is_home_workspace(&registry, "ghost"));
    }
}
