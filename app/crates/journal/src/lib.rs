//! Typed log events: code returns or hands over an [`Event`] value instead of logging inline,
//! and each crate turns events into tracing events in one place, [`sink!`], under its own target.

mod event;
mod level;
mod sink;

pub use event::Event;
pub use level::Level;
#[doc(hidden)]
pub use tracing;
