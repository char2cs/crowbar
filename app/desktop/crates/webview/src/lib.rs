//! The one platform crate: native web views embedded as children of a window.
//!
//! [`WebViewHost::native`] starts the backend of the current platform and
//! [`WebViewHost::create`] returns a [`WebViewHandle`] with `set_bounds`, `show`, `hide`,
//! `eval`, `post_message` and `destroy`.
//!
//! * Linux: web views are X11 children of the GPUI window. `WebKitGTK` is single-threaded, so a
//!   dedicated GTK thread owns every view. Core panes are related views that share one web
//!   process; each extension gets its own web context and process. Contexts live as long as
//!   the host, so destroying views never leaks a network process.
//! * macOS: web views are created on the main thread with the window's own `NSView`; each
//!   extension gets its own data store (macOS 14+). `snapshot` returns no pixels yet, so overlays
//!   hide the pane there; the macOS capture is tracked in docs/plans/2026-11-14-gpui-shell-pane-host.md.
//!   This path compiles on every CI run and is exercised by the macOS job only.
//!
//! Pages talk to the shell through `window.ipc.postMessage` (one [`IpcHandler`] per view, so
//! the closure is the sender's identity) and through the custom scheme served by a
//! [`ProtocolHandler`]; the shell talks to pages through [`WebViewHandle::post_message`].
//!
//! ```
//! use webview::{Bounds, Visibility, WebViewConfig, WebViewHost, WebViewKind};
//! use webview::{MockPlatform, ParentWindow};
//! use std::rc::Rc;
//! use std::sync::Arc;
//!
//! let platform = Rc::new(MockPlatform::default());
//! let host = WebViewHost::with_platform(platform.clone());
//! let view = host.create_in(
//!     ParentWindow::X11(1),
//!     WebViewConfig {
//!         url: "crowbar://local/chat/".into(),
//!         bounds: Bounds { x: 0.0, y: 0.0, width: 400.0, height: 300.0 },
//!         kind: WebViewKind::Core,
//!         visibility: Visibility::Shown,
//!         background: None,
//!         initialization_script: None,
//!         on_ipc: Arc::new(|_| {}),
//!         on_page: None,
//!     },
//! )?;
//! view.hide()?;
//! assert_eq!(platform.calls().len(), 2);
//! # Ok::<(), webview::Error>(())
//! ```

#[cfg(target_os = "linux")]
mod adapters;
mod bitmap_rows;
mod bounds;
mod browsing_data;
mod builder_support;
#[cfg(feature = "mock")]
mod call;
mod color;
mod error;
mod host_config;
#[cfg(feature = "mock")]
mod mock_page;
#[cfg(feature = "mock")]
mod mock_platform;
#[cfg(feature = "mock")]
mod mock_views;
mod page_control;
mod page_event;
mod parent_window;
mod platform;
mod protocol_handler;
mod protocol_request;
mod protocol_response;
mod row_layout;
mod snapshot;
#[cfg(test)]
mod test_support;
mod view_capture;
mod view_focus;
mod view_layout;
mod view_lifecycle;
mod visibility;
mod web_view_config;
mod web_view_handle;
mod web_view_host;
mod web_view_id;
mod web_view_kind;
mod web_view_page;

#[cfg(target_os = "linux")]
mod bounded_wait;
#[cfg(target_os = "linux")]
mod create_reply;
#[cfg(target_os = "linux")]
mod gtk_state;
#[cfg(target_os = "linux")]
mod gtk_view_ops;
#[cfg(target_os = "linux")]
mod linux_page;
#[cfg(target_os = "linux")]
mod linux_platform;
#[cfg(target_os = "linux")]
mod linux_views;
#[cfg(target_os = "linux")]
mod live_view;
#[cfg(target_os = "linux")]
mod placed;
#[cfg(target_os = "linux")]
mod surface_pixels;
#[cfg(target_os = "linux")]
mod xlib_parent;

#[cfg(target_os = "macos")]
mod appkit_parent;
#[cfg(target_os = "macos")]
mod macos_data_store;
#[cfg(target_os = "macos")]
mod macos_platform;

pub use bounds::Bounds;
pub use browsing_data::BrowsingData;
pub use color::Color;
pub use error::{Error, Result};
pub use host_config::HostConfig;
pub use page_control::PageControl;
pub use page_event::{PageEvent, PageHandler};
pub use parent_window::ParentWindow;
pub use platform::Platform;
pub use protocol_handler::{ProtocolHandler, ProtocolResponder};
pub use protocol_request::ProtocolRequest;
pub use protocol_response::ProtocolResponse;
pub use snapshot::Snapshot;
pub use view_capture::ViewCapture;
pub use view_focus::ViewFocus;
pub use view_layout::ViewLayout;
pub use view_lifecycle::ViewLifecycle;
pub use visibility::Visibility;
pub use web_view_config::{IpcHandler, WebViewConfig};
pub use web_view_handle::{RECEIVER, WebViewHandle};
pub use web_view_host::WebViewHost;
pub use web_view_id::WebViewId;
pub use web_view_kind::WebViewKind;

#[cfg(feature = "mock")]
pub use call::Call;
#[cfg(feature = "mock")]
pub use mock_platform::MockPlatform;
