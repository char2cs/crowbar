//! The workspace family: its branch icon, its inline-rename input, its
//! switcher and its tree. `workspace_tree`'s own row (`workspace_tree_item`)
//! is here too; the row family it composes with (`pending_create_row`,
//! `repo_section`) is one directory over in [`super::rows`] and
//! [`super::repo`] respectively — see [`super::rows`]'s module docs for the
//! P3.61 dependency chain this cluster is one link of.

// `workspace_branch_icon` is unflattened for the same reason as the rest. Its
// `Status`, `Glyph` and `ID` would collide with
// `super::rows::file_tree_row`'s `GitStatus` and
// `super::rows::git_status_row`'s `ID_ICON` neighbours under a different
// shape of the same mistake, and it reuses
// `crate::primitives::flicker_spinner::CallSite::WorkspaceIcon` directly
// rather than reimplementing the spinner it wraps — see its module docs.
pub mod workspace_branch_icon;
// `workspace_switcher` (P3.58) is unflattened for the same reason as the
// rest, and carries **no anchor of its own** — see the module docs. Its
// `Row` reuses `crate::primitives::autocomplete::ID_ITEM` directly (the one
// real, already-registered anchor for a `command-item`'s content, which
// `autocomplete.rs` itself leaves opaque) rather than fabricating a second
// id nothing on the React side would ever produce.
pub mod workspace_switcher;
// `workspace_inline_input` (P3.62) is unflattened for the same reason as the
// rest — its `ID_ROOT`/`ID_FIELD`/`Kind` would read ambiguously without the
// module in front of them. Its `<input>` field is
// `crate::primitives::input`'s own "box only, no text field" finding one
// door over; see the module docs.
pub mod workspace_inline_input;
// `workspace_tree_item` → `super::repo::repo_section` → `workspace_tree`
// (P3.61) is `native/mapping/layout-denominator.md` §8's Cluster 8 — a
// genuine dependency chain, landed together by one item, and the reason this
// cluster crosses the `workspace`/`repo`/`rows` boundary at all. Each is
// unflattened for the usual reason (`ID_ROOT`/`ID_LABEL` would collide with
// other surfaces' generic-sounding names otherwise). Neither calls
// `AnchorSink::root`; both are always composed — by a parent, or by
// `workspace_tree_item` itself, recursively — and use `AnchorSink::boxed` for
// their own root, `workspace_branch_icon`'s own precedent. See each module's
// own docs, and [`super::rows`]'s module docs for `workspace_tree`'s own
// place in the chain.
pub mod workspace_tree;
pub mod workspace_tree_item;
