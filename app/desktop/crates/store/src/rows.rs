//! The chats tree turned into sidebar rows: emitter, look, project, pending, scope items, workspace view, repo marks and the row of a view tab's chat.

pub(crate) mod chat_member;
mod emitter;
mod forest;
pub(crate) mod look;
pub(crate) mod pending;
pub(crate) mod project;
pub(crate) mod repo_mark;
pub(crate) mod scope_items;
pub(crate) mod target;
pub(crate) mod workspace_view;

pub(crate) use look::title_of;
pub(crate) use scope_items::default_workspace;
