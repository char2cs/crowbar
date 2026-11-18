//! The branch import dialog of the Crowbar shell.
//!
//! [`BranchImportView`] lists the remote branches of one repository from a [`store::BranchImport`],
//! lets the user pick branches and lock some of them, and starts the batch import. It emits
//! [`ImportEvent::Close`] when it is done or dismissed.

mod branch_import_view;
mod branch_source;
mod filter;
mod hint;
mod import_event;
mod look;
mod row;
mod row_choice;
mod selection;

pub use branch_import_view::BranchImportView;
pub use import_event::ImportEvent;
