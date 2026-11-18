//! The three panels and their editors: chats, the Files explorer, changes, the create-space panel and inline edits.

mod changes;
mod chats;
mod create_space;
mod edit;
mod explorer;
mod message;
mod scrolls;

pub(crate) use changes::MergeDraft;
pub(crate) use chats::{DragState, RowBounds, foot_menu, row_menu};
pub(crate) use create_space::SpaceDraft;
pub(crate) use edit::InlineEdit;
pub(crate) use explorer::FileSearch;
pub(crate) use scrolls::Scrolls;
