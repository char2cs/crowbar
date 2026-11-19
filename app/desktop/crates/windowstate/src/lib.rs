//! Remembers the shell window between runs.
//!
//! [`WindowStore`] reads and writes a [`WindowState`] file; [`WindowState::placement`] turns a
//! remembered state into a [`Placement`] that is guaranteed to be on a connected display and
//! not smaller than 640×480, so a window left on an unplugged monitor comes back visible.

mod error;
mod placement;
mod rect;
mod window_state;
mod window_store;

pub use error::Error;
pub use placement::Placement;
pub use rect::Rect;
pub use window_state::{DEFAULT_SIZE, WindowState};
pub use window_store::WindowStore;
