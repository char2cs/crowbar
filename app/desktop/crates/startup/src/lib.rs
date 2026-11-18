//! The full-screen native startup and data-migration view of the Crowbar shell.
//!
//! Entry: [`View`]. The shell feeds it [`Input`]s derived from the daemon
//! supervisor's events, answers its [`Command`]s through [`Actions`], and removes
//! it when it emits [`Event::HandedOver`]. [`Machine`] is the pure state machine
//! underneath and can be driven without a window.

mod actions;
mod command;
mod machine;
mod screens;
mod test_support;
mod view;

pub use actions::Actions;
pub use command::Command;
pub use machine::{AppPhase, HaltReason, Input, Machine, MigrationStep, Outage, Screen, Timing};
pub use view::{Clock, Event, MonotonicClock, View};
