//! The Add Host and Add a device dialogs of the Crowbar shell.
//!
//! [`View`] walks one [`store::PairingSession`] through its stages: the host's name and
//! address, the fingerprint to compare with `crowbar pair`, then the one-time code. It emits
//! [`Event::Paired`] when the host is paired and [`Event::Close`] when it is dismissed.
//! [`OfferView`] shows the one-time code another device pairs with this computer.

mod action;
mod event;
mod look;
mod offer_view;
mod source;
mod stage_copy;
mod stage_widgets;
mod view;

pub use event::Event;
pub use offer_view::OfferView;
pub use view::View;
