//! Wire types between the Crowbar shell, its panes and the daemon transport.
//!
//! The crate owns three things:
//!
//! * identifiers shared by every crate ([`HostId`], [`PaneId`], [`PaneTypeId`], ...),
//! * the `sdk@1` control frames ([`PaneFrame`] and [`ShellFrame`]) with their JSON text
//!   encoding, the 64 KB [`PaneState`] cap and the 256 KB frame cap; every string and map a
//!   frame carries is a validated, bounded type,
//! * the [`DaemonTransport`] port the shell uses to reach the daemon.
//!
//! It has no GUI, web view or async runtime dependency, so desktop and mobile shells can
//! share it.
//!
//! ```
//! use protocol::{PaneFrame, PaneTitle, decode_pane_frame, encode};
//!
//! let text = r#"{"sdk":1,"kind":"pane.setTitle","title":"Chat"}"#;
//! let frame = decode_pane_frame(text)?;
//! assert_eq!(frame, PaneFrame::SetTitle { title: PaneTitle::new("Chat")? });
//! assert_eq!(encode(&frame)?, text);
//! # Ok::<(), protocol::Error>(())
//! ```

mod app_phase;
mod attachment_id;
mod attachment_type;
mod byte_counter;
mod command_id;
mod daemon_request;
mod daemon_response;
mod daemon_transport;
mod display_text;
mod envelope;
mod error;
mod error_code;
mod event_name;
mod extension_id;
mod extension_status;
mod external_url;
mod fs_path;
mod host_id;
mod http_method;
mod key_binding;
mod key_chord;
mod lifecycle;
mod method_name;
mod pane_frame;
mod pane_id;
mod pane_kind;
mod pane_origin;
mod pane_state;
mod pane_title;
mod pane_type_id;
mod params;
mod protocol;
mod relay_endpoint;
mod relay_scope;
mod request_id;
mod request_target;
mod rpc_error;
mod rpc_outcome;
mod rpc_response;
mod rpc_response_wire;
mod sdk_version;
mod setting_key;
mod setting_values;
mod shell_frame;
mod space_id;
mod status_kind;
mod string_id;
mod theme_tokens;

pub use app_phase::AppPhase;
pub use attachment_id::AttachmentId;
pub use attachment_type::AttachmentType;
pub use command_id::CommandId;
pub use daemon_request::{DaemonRequest, MAX_HEADER_BYTES, MAX_HEADERS};
pub use daemon_response::DaemonResponse;
pub use daemon_transport::{DaemonTransport, Responder};
pub use display_text::{DisplayText, MAX_DISPLAY_TEXT_BYTES};
pub use error::{Error, Result};
pub use error_code::ErrorCode;
pub use event_name::EventName;
pub use extension_id::ExtensionId;
pub use extension_status::ExtensionStatus;
pub use external_url::ExternalUrl;
pub use fs_path::FsPath;
pub use host_id::HostId;
pub use http_method::HttpMethod;
pub use key_binding::KeyBinding;
pub use key_chord::KeyChord;
pub use lifecycle::Lifecycle;
pub use method_name::MethodName;
pub use pane_frame::PaneFrame;
pub use pane_id::PaneId;
pub use pane_kind::PaneKind;
pub use pane_origin::PaneOrigin;
pub use pane_state::{MAX_STATE_BYTES, PaneState};
pub use pane_title::{MAX_TITLE_BYTES, PaneTitle};
pub use pane_type_id::PaneTypeId;
pub use params::{MAX_PARAMS, Params};
pub use protocol::{MAX_FRAME_BYTES, decode_pane_frame, decode_shell_frame, encode};
pub use relay_endpoint::RelayEndpoint;
pub use relay_scope::RelayScope;
pub use request_id::RequestId;
pub use request_target::{MAX_TARGET_BYTES, RequestTarget};
pub use rpc_error::RpcError;
pub use rpc_outcome::RpcOutcome;
pub use rpc_response::RpcResponse;
pub use sdk_version::{SDK_VERSION, SdkVersion};
pub use setting_key::SettingKey;
pub use setting_values::{MAX_SETTING_VALUES, SETTINGS_FRAME_VERSION, SettingValues};
pub use shell_frame::ShellFrame;
pub use space_id::SpaceId;
pub use status_kind::StatusKind;
pub use theme_tokens::{MAX_THEME_TOKENS, ThemeTokens};
