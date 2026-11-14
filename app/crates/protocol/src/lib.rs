//! Wire types between the Crowbar shell, its panes and the daemon transport.
//!
//! The crate owns three things:
//!
//! * identifiers shared by every crate ([`HostId`], [`PaneId`], [`PaneTypeId`], ...),
//! * the `sdk@1` control frames ([`PaneFrame`] and [`ShellFrame`]) with their JSON text
//!   encoding and the 64 KB [`PaneState`] cap,
//! * the [`DaemonTransport`] port the shell uses to reach the daemon.
//!
//! It has no GUI, web view or async runtime dependency, so desktop and mobile shells can
//! share it.
//!
//! ```
//! use protocol::{PaneFrame, decode_pane_frame, encode};
//!
//! let text = r#"{"sdk":1,"kind":"pane.setTitle","title":"Chat"}"#;
//! let frame = decode_pane_frame(text)?;
//! assert_eq!(frame, PaneFrame::SetTitle { title: "Chat".into() });
//! assert_eq!(encode(&frame)?, text);
//! # Ok::<(), protocol::Error>(())
//! ```

mod app_phase;
mod byte_counter;
mod daemon_request;
mod daemon_response;
mod daemon_transport;
mod envelope;
mod error;
mod error_code;
mod extension_id;
mod extension_status;
mod host_id;
mod key_binding;
mod lifecycle;
mod pane_frame;
mod pane_id;
mod pane_kind;
mod pane_state;
mod pane_type_id;
mod params;
mod protocol;
mod relay_endpoint;
mod relay_scope;
mod request_id;
mod rpc_error;
mod shell_frame;
mod space_id;
mod status_kind;
mod string_id;
mod version;

pub use app_phase::AppPhase;
pub use daemon_request::DaemonRequest;
pub use daemon_response::DaemonResponse;
pub use daemon_transport::{DaemonTransport, Responder};
pub use error::{Error, Result};
pub use error_code::ErrorCode;
pub use extension_id::ExtensionId;
pub use extension_status::ExtensionStatus;
pub use host_id::HostId;
pub use key_binding::KeyBinding;
pub use lifecycle::Lifecycle;
pub use pane_frame::PaneFrame;
pub use pane_id::PaneId;
pub use pane_kind::PaneKind;
pub use pane_state::{MAX_STATE_BYTES, PaneState};
pub use pane_type_id::PaneTypeId;
pub use params::Params;
pub use protocol::{MAX_FRAME_BYTES, decode_pane_frame, decode_shell_frame, encode};
pub use relay_endpoint::RelayEndpoint;
pub use relay_scope::RelayScope;
pub use request_id::RequestId;
pub use rpc_error::RpcError;
pub use shell_frame::ShellFrame;
pub use space_id::SpaceId;
pub use status_kind::StatusKind;
pub use version::{SDK_MAJOR, SDK_VERSION};
