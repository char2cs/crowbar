//! What a web view is: its config, id, kind, bounds, colour, visibility, parent window, snapshot and page events.

mod bounds;
mod color;
mod page_event;
mod parent_window;
mod snapshot;
mod visibility;
mod web_view_config;
mod web_view_id;
mod web_view_kind;

pub use bounds::Bounds;
pub use color::Color;
pub use page_event::{PageEvent, PageHandler};
pub use parent_window::ParentWindow;
pub use snapshot::Snapshot;
pub use visibility::Visibility;
pub use web_view_config::{IpcHandler, WebViewConfig};
pub use web_view_id::WebViewId;
pub use web_view_kind::WebViewKind;
