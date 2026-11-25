//! The Add a space and Add repository dialogs of the Crowbar shell.
//!
//! [`View`] takes a folder path, typed or browsed, and a name, and asks its owner to add them
//! with [`ImportEvent::Add`]. It shows "Adding…" while its folder is being added and closes when
//! the owner reports the folder [`View::added`]; a failure leaves it open for another try.

mod folder_text;
mod import_event;
mod kind;
mod look;
mod view;

pub use import_event::ImportEvent;
pub use kind::Kind;
pub use view::{Adding, View};
