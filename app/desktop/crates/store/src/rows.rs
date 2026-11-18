//! The chats tree turned into sidebar rows: emitter, look, project, pending, scope items, workspace view and repo marks.

mod forest;
mod look;
pub(crate) mod pending;
pub(crate) mod project;
pub(crate) mod repo_mark;
mod row_emitter;
pub(crate) mod scope_items;
pub(crate) mod workspace_view;

pub(crate) use look::title_of;
pub(crate) use scope_items::default_workspace;
