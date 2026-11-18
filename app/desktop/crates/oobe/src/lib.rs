//! The first-run screens of the Crowbar shell.
//!
//! Entry: [`View`]. It shows the brand and its tagline, the system check, the invitation to
//! add a first space, or the invitation to add a host, over the brand gradient. It emits an
//! [`Event`] when the user asks for the folder picker or the Add Host dialog.

mod backdrop;
mod check_source;
mod check_status;
mod event;
mod look;
mod pill_button;
mod screen;
mod test_support;
mod view;

pub use event::Event;
pub use screen::Screen;
pub use view::View;
