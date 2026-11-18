//! The branch import dialog of the Crowbar shell.
//!
//! [`BranchImportView`] lists the remote branches of one repository from a [`shell_data::BranchImport`],
//! lets the user pick branches and lock some of them, and starts the batch import. It emits
//! [`ImportEvent::Close`] when it is done or dismissed.

mod branch_filter;
mod branch_import_view;
mod branch_row;
mod branch_source;
mod import_event;
mod import_hint;
mod row_choice;
mod row_look;
mod selection;

pub use branch_import_view::BranchImportView;
pub use import_event::ImportEvent;
