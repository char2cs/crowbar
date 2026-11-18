//! The sidebar column's geometry: panel width, dock side, visibility, presentation, peek zone, mode and the preferences file.

mod dock_side;
mod metrics;
mod mode;
mod panel;
mod peek_zone;
mod preferences;
mod presentation;
mod separator;
mod visibility;

pub use dock_side::DockSide;
pub(crate) use metrics::REMOVAL_DRAIN_SECONDS;
pub use metrics::{HEADER_HEIGHT, PEEK_INSET};
pub use mode::Mode;
pub use panel::Panel;
pub use peek_zone::PeekZone;
pub use preferences::Preferences;
pub use presentation::Presentation;
pub use separator::{KeyStep, ResizeDrag, ResizeKey, Resizing};
pub use visibility::Visibility;
