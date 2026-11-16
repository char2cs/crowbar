//! The first-run screens of the Crowbar shell.
//!
//! Entry: [`OobeView`]. It shows the brand and its tagline, the system check, the invitation to
//! add a first space, or the invitation to add a host, over the brand gradient. It emits an
//! [`OobeEvent`] when the user asks for the folder picker or the Add Host dialog.

mod backdrop;
mod check_source;
mod check_status;
mod look;
mod oobe_event;
mod oobe_screen;
mod oobe_view;
mod pill_button;
mod test_support;

pub use oobe_event::OobeEvent;
pub use oobe_screen::OobeScreen;
pub use oobe_view::OobeView;
