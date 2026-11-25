//! The stock windows that float above an owner window: one per [`crate::Float`] kind, each
//! showing its owner's layers of that kind.

mod drag_forward;
mod float_host;
mod opening;
mod owner;
mod owners;
mod registry;
mod root;
mod shaped;
mod slot;
mod stack;

pub use float_host::FloatHost;
pub use float_host::float_region::FloatRegion;
pub use registry::install_float_host;
pub use shaped::Shaped;

pub(crate) use opening::place;
pub(crate) use owners::Owners;
pub use owners::{holds_keys, open_float_window};
pub(crate) use registry::Registry;
pub(crate) use slot::Slot;
