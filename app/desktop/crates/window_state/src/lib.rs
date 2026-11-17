//! Remembers the shell window between runs.
//!
//! [`WindowStore`] reads and writes a [`WindowState`] file; [`WindowState::placement`] turns a
//! remembered state into a [`Placement`] that is guaranteed to be on a connected display and
//! not smaller than [`MIN_SIZE`], so a window left on an unplugged monitor comes back visible.

mod adapters;
mod error;
mod placement;
mod rect;
mod state_file;
mod test_support;
mod window_state;
mod window_store;

pub use error::Error;
pub use placement::Placement;
pub use rect::Rect;
pub use window_state::{DEFAULT_SIZE, MIN_SIZE, WindowState};
pub use window_store::WindowStore;
