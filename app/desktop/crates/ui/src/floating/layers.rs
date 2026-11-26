//! The stock pop-up window that floats above an owner window, showing its owner's passive
//! layers: tooltips and toasts.

mod float_host;
mod opening;
mod owner;
mod owners;
mod registry;
mod root;
mod shaped;
mod stack;

pub use float_host::FloatHost;
pub use float_host::float_region::FloatRegion;
pub use registry::install_float_host;
pub use shaped::Shaped;

pub(crate) use opening::place;
pub(crate) use owners::Owners;
