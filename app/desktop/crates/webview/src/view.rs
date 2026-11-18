//! What a web view is: its config, id, kind, bounds, colour, visibility, parent window, snapshot and page events.

mod bounds;
mod color;
mod config;
mod id;
mod kind;
mod page_event;
mod parent_window;
mod snapshot;
mod visibility;

pub use bounds::Bounds;
pub use color::Color;
pub use config::{Config, IpcHandler};
pub use id::Id;
pub use kind::Kind;
pub use page_event::{PageEvent, PageHandler};
pub use parent_window::ParentWindow;
pub use snapshot::Snapshot;
pub use visibility::Visibility;
