//! The full-screen native startup and data-migration view of the Crowbar shell.
//!
//! Entry: [`StartupView`]. The shell feeds it [`StartupInput`]s derived from the daemon
//! supervisor's events, answers its [`StartupCommand`]s through [`StartupActions`], and removes
//! it when it emits [`StartupEvent::HandedOver`]. [`StartupMachine`] is the pure state machine
//! underneath and can be driven without a window.

mod command_button;
mod detail_box;
mod halt_panel;
mod halt_reason;
mod handover;
mod migration_step;
mod monotonic_clock;
mod outage;
mod outage_panel;
mod startup_actions;
mod startup_clock;
mod startup_command;
mod startup_event;
mod startup_input;
mod startup_machine;
mod startup_screen;
mod startup_timing;
mod startup_view;
mod test_support;
mod waiting_panel;
mod wordmark;

pub use halt_reason::HaltReason;
pub use migration_step::MigrationStep;
pub use monotonic_clock::MonotonicClock;
pub use outage::Outage;
pub use startup_actions::StartupActions;
pub use startup_clock::StartupClock;
pub use startup_command::StartupCommand;
pub use startup_event::StartupEvent;
pub use startup_input::StartupInput;
pub use startup_machine::StartupMachine;
pub use startup_screen::StartupScreen;
pub use startup_timing::StartupTiming;
pub use startup_view::StartupView;
