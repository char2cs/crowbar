//! The one platform crate: native web views embedded as children of a window.
//!
//! [`Host::native`] starts the backend of the current platform and
//! [`Host::create`] returns a [`Handle`] with `set_bounds`, `show`, `hide`,
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
//! [`ProtocolHandler`]; the shell talks to pages through [`Handle::post_message`].
//!
//! ```
//! use webview::{Bounds, Visibility, Config, Host, Kind};
//! use webview::{MockPlatform, ParentWindow};
//! use std::rc::Rc;
//! use std::sync::Arc;
//!
//! let platform = Rc::new(MockPlatform::default());
//! let host = Host::with_platform(platform.clone());
//! let view = host.create_in(
//!     ParentWindow::X11(1),
//!     Config {
//!         url: "crowbar://local/chat/".into(),
//!         bounds: Bounds { x: 0.0, y: 0.0, width: 400.0, height: 300.0 },
//!         kind: Kind::Core,
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

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
compile_error!("webview hosts web views on Linux and macOS only");

mod backend;
mod error;
mod host;
mod scheme;
#[cfg(test)]
mod test_support;
mod view;

pub use backend::{
    BrowsingData, HostConfig, PageControl, Platform, ViewCapture, ViewFocus, ViewLayout,
    ViewLifecycle,
};
pub use error::{Error, Result};
pub use host::{Handle, Host};
pub use scheme::{ProtocolHandler, ProtocolRequest, ProtocolResponder, ProtocolResponse};
pub use view::{
    Bounds, Color, Config, Id, IpcHandler, Kind, PageEvent, PageHandler, ParentWindow, Snapshot,
    Visibility,
};

#[cfg(feature = "mock")]
pub use backend::Call;
#[cfg(feature = "mock")]
pub use backend::MockPlatform;
