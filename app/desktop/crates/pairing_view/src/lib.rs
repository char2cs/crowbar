//! The Add Host dialog of the Crowbar shell.
//!
//! [`PairingView`] walks one [`shell_data::PairingSession`] through its stages: the host's name and
//! address, the fingerprint to compare with `crowbar pair`, then the one-time code. It emits
//! [`PairingEvent::Paired`] when the host is paired and [`PairingEvent::Close`] when it is dismissed.

mod look;
mod pairing_action;
mod pairing_event;
mod pairing_source;
mod pairing_view;
mod stage_copy;
mod stage_widgets;

pub use pairing_event::PairingEvent;
pub use pairing_view::PairingView;
