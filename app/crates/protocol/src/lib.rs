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

mod control;
mod error;
mod identity;
mod port;
mod protocol;
mod string_id;

pub use control::{
    DisplayText, ErrorCode, EventName, ExtensionStatus, ExternalUrl, FsPath, KeyBinding, KeyChord,
    Lifecycle, MAX_DISPLAY_TEXT_BYTES, MAX_PARAMS, MAX_SETTING_VALUES, MAX_STATE_BYTES,
    MAX_THEME_TOKENS, MAX_TITLE_BYTES, MethodName, PANE_SETTING_KEYS, PaneFrame, PaneState,
    PaneTitle, Params, RequestId, RpcError, RpcOutcome, RpcResponse, SDK_VERSION,
    SETTINGS_FRAME_VERSION, SdkVersion, SettingKey, SettingValues, ShellFrame, StatusKind,
    ThemeTokens, ViewedFile,
};
pub use error::{Error, Result};
pub use identity::{
    CommandId, ExtensionId, HostId, PaneId, PaneKind, PaneOrigin, PaneTypeId, SpaceId,
};
pub use port::{
    DaemonRequest, DaemonResponse, DaemonTransport, HttpMethod, MAX_HEADER_BYTES, MAX_HEADERS,
    MAX_TARGET_BYTES, RelayEndpoint, RelayScope, RequestTarget, Responder,
};
pub use protocol::{MAX_FRAME_BYTES, decode_pane_frame, decode_shell_frame, encode};
