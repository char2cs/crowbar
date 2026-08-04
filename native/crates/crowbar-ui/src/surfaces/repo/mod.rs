//! The repo family: its avatar, its icon popover, its import dialog and its
//! sidebar section. `repo_section` composes `super::rows::pending_create_row`
//! and this crate's own [`workspace_tree_item`](super::workspace::workspace_tree_item)
//! — see [`super::rows`]'s module docs for the P3.61 chain this cluster is
//! part of.

// `repo_avatar` is unflattened for the same reason as the rest. Its `ID`,
// `CONTENT_SIZED` and `LINE_SIZED` would collide outright, and its `Size` —
// `sm`/`lg`/`xl`, no `md` — would read as a table belonging to no component
// without the module in front of it: `crate::primitives::avatar::CallSite`'s
// three bundles answer a different question. P3.54 landed `data-oracle-id`s
// on this file and on `super::workspace::workspace_branch_icon`'s — see each
// module's own docs.
pub mod repo_avatar;
// `repo_icon_popover` (P3.58) is unflattened for the same reason as the rest:
// its own `ID_TRIGGER`/`ID_POPUP` and its bespoke `Kind`/`ImageState` reuse
// would read as belonging to no component without the module in front of
// them, and its trigger's own `h-5 w-5 rounded-md` box is a genuinely
// different shape from `repo_avatar::Size::Lg`'s `rounded-sm` one — see the
// module docs for why it is not built by calling `RepoAvatar::render`.
pub mod repo_icon_popover;
// `repo_import_dialog` (P3.51) is unflattened for the same reason as the
// rest — see `super::detach_holder_modal`'s own module doc comment for the
// shared reasoning; the two are one P3.51 pair split across a directory
// boundary by the `repo_`-prefix grouping rule.
pub mod repo_import_dialog;
// `repo_section` (P3.61) — see [`super::rows`]'s module docs for the
// Cluster 8 dependency chain it sits in.
pub mod repo_section;
