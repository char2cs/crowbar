//! Workspace/path scoping — spec §4.2's `"workspace scoping"` bucket of
//! `crowbar-core`.
//!
//! Ported from `web/src/lib/workspace-scope.ts`, `workspace-scope-url.ts`,
//! `workspace/placeholder.ts`, `workspace/branch-workspace.ts` and
//! `features/workspace/lib/keep-alive-policy.ts` — the five files
//! `native/mapping/tier-a-denominator.md` §"Workspace scoping" classifies as
//! Tier A core for this area (261 lines, 32 existing test cases). Everything
//! else the survey found under `web/src/lib/workspace/` and
//! `features/workspace/lib/` is Phase 4 (reads a zustand store's `.getState()`,
//! does async fetch + cache, or is a webview-only DOM/CSS mounting strategy)
//! and is deliberately not here — see the survey for the file-by-file case.
//!
//! # A shape difference from the TS source, and why
//!
//! [`scope::WorkspaceScope`]'s registry (`_scopes`/`_activeWorkspaceId` in the
//! TS) is a **module-level global** there — a deliberate "dependency-free
//! module singleton" per that file's own comment, chosen so lightweight
//! files/git/lsp/terminal URL builders can resolve a scope without importing
//! the heavy editor/Monaco workspace-store graph.
//!
//! `crowbar-core` has no equivalent problem to solve: nothing here is a module
//! system where importing a type drags in a graph of stores, and §7.1 already
//! gives this exact shape a home — *"domain state lives in `crowbar-core` as
//! plain Rust; `crowbar-state` holds the `Entity<T>` wrapper … If a store's
//! logic can be tested without gpui, it belongs in core."* A Rust `static` /
//! `OnceLock<Mutex<_>>` singleton here would not simplify anything the TS
//! singleton was working around — it would just relocate the same "hidden
//! global mutable state" problem D2 exists to get *out* of `crowbar-core`, one
//! layer down. So [`scope::WorkspaceScopeRegistry`] is an owned, plain struct;
//! the caller (eventually `crowbar-state`, behind an `Entity<T>`) holds the one
//! instance that plays the role `_scopes`/`_activeWorkspaceId` played as
//! globals. Every other function and type here is a direct, same-shape port.
//!
//! # Two behaviours worth stating rather than silently reproducing
//!
//! * [`scope::WorkspaceScopeRegistry::get_workspace_scope`]: the TS body is
//!   `const id = wsId ?? _activeWorkspaceId; if (!id) return null; …`. `??`
//!   (nullish coalescing) does **not** treat `""` as absent — only
//!   `null`/`undefined` — but the very next line's `if (!id)` **does**, because
//!   `""` is falsy. Net effect: an explicitly-passed empty-string `wsId`
//!   resolves to "no scope", full stop, even if `_scopes.get('')` would
//!   otherwise have found something. This can't happen through the route
//!   parser (a capture group is never empty — see [`scope::parse_workspace_scope_from_path`]),
//!   so it is a defended-but-unreachable edge in practice; the port keeps it
//!   because the alternative is a silent behaviour change from a Rust
//!   `Option`'s more literal semantics.
//! * [`placeholder::is_placeholder_workspace`] and
//!   [`placeholder::placeholder_reason`]: `!ws.localPath`, `if (ws.heldByPath)`
//!   and `if (ws.lastError)` are JS truthy checks, so an explicit empty string
//!   on the wire is indistinguishable from the field being absent. Both are
//!   ported with the same equivalence (`None` or `Some("")` both count as
//!   "not set") rather than the narrower `Option::is_none`, which would treat
//!   `Some(String::new())` as present and change behaviour for a DTO the
//!   generator is free to produce that way (`#[serde(skip_serializing_if =
//!   "Option::is_none")]` only omits `None`, never an empty string).
//!
//! # A hand-written type this port did *not* need
//!
//! `placeholder.rs` and `branch.rs` operate on the same four `Workspace`
//! fields (`localPath`, `heldByPath`, `branch`, `lastError`/`id`) the TS side
//! hand-declares on its own `Workspace` interface (`web/src/lib/store/sidebar.ts`).
//! `crowbar-proto`'s generated `api_v0_dto::WorkspaceDTO` already carries all
//! four under the same names (§9.2), so both modules take `&WorkspaceDTO`
//! directly instead of re-declaring a duplicate type — the case §9.2 exists to
//! prevent. One consequence: [`branch::find_workspace_for_branch`] takes a
//! `&[WorkspaceDTO]` slice rather than a hand-rolled `Repo` type, because the
//! wire has no `Repo`-with-embedded-`workspaces` shape — `RepoDTO` and
//! `WorkspaceDTO` are separate broadcast channels the frontend store
//! aggregates client-side (Phase 4); the function never touched anything on
//! `Repo` besides that array anyway, so nothing is lost.

pub mod branch;
pub mod keep_alive;
pub mod placeholder;
pub mod scope;
pub mod scope_url;
