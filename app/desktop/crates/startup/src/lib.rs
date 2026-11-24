//! The halt gate of the Crowbar shell: what replaces the window's content once the daemon halts.
//!
//! Entry: [`View`]. The shell feeds it every [`Input`] derived from the daemon supervisor's
//! events and answers its [`Command`]s through [`Actions`]; the gate covers the window from a
//! halt until the daemon is started again.

mod actions;
mod command;
mod report;
mod screens;
mod test_support;
mod view;

pub use actions::Actions;
pub use command::Command;
pub use report::{HaltReason, Input, MigrationStep, Outage};
pub use view::View;
