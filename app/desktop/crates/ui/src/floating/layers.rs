//! The stock pop-up window that floats above an owner window, showing its owner's passive
//! layers: tooltips and toasts. It never takes the keyboard.

mod float_host;
mod float_region;
mod opening;
mod owner;
mod owners;
mod painted;
mod registry;
mod root;
mod shaped;

pub use float_host::FloatHost;
pub use float_region::FloatRegion;
pub use registry::install_float_host;
pub use shaped::Shaped;

pub(crate) use opening::place;
pub(crate) use owners::Owners;
