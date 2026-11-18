//! The full-screen native startup and data-migration view of the Crowbar shell.
//!
//! Entry: [`StartupView`]. The shell feeds it [`StartupInput`]s derived from the daemon
//! supervisor's events, answers its [`StartupCommand`]s through [`StartupActions`], and removes
//! it when it emits [`StartupEvent::HandedOver`]. [`StartupMachine`] is the pure state machine
//! underneath and can be driven without a window.

mod screens;
mod startup_actions;
mod startup_command;
mod startup_machine;
mod startup_view;
mod test_support;

pub use startup_actions::StartupActions;
pub use startup_command::StartupCommand;
pub use startup_machine::{
    AppPhase, HaltReason, MigrationStep, Outage, StartupInput, StartupMachine, StartupScreen,
    StartupTiming,
};
pub use startup_view::{MonotonicClock, StartupClock, StartupEvent, StartupView};
