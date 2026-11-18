//! The Add Host dialog of the Crowbar shell.
//!
//! [`View`] walks one [`store::PairingSession`] through its stages: the host's name and
//! address, the fingerprint to compare with `crowbar pair`, then the one-time code. It emits
//! [`Event::Paired`] when the host is paired and [`Event::Close`] when it is dismissed.

mod action;
mod event;
mod look;
mod source;
mod stage_copy;
mod stage_widgets;
mod view;

pub use event::Event;
pub use view::View;
