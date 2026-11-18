//! The full-screen native startup and data-migration view of the Crowbar shell.
//!
//! Entry: [`View`]. The shell feeds it [`Input`]s derived from the daemon
//! supervisor's events, answers its [`Command`]s through [`Actions`], and removes
//! it when it emits [`Event::HandedOver`]. A pure state machine underneath turns the inputs
//! into the screen to show and never reads a clock.

mod actions;
mod command;
mod machine;
mod screens;
mod test_support;
mod view;

pub use actions::Actions;
pub use command::Command;
pub use machine::{AppPhase, HaltReason, Input, MigrationStep, Outage, Timing};
pub use view::{Clock, Event, MonotonicClock, View};
