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
    Breakpoint, DisplayText, DocumentPath, ErrorCode, EventName, ExtensionStatus, ExternalUrl,
    FsPath, KeyBinding, KeyChord, Lifecycle, MethodName, OpenMode, PANE_SETTING_KEYS, PairedHost,
    PaneFrame, PaneOpen, PaneState, PaneTitle, PaneToast, Params, Precedence, RequestId, RpcError,
    RpcOutcome, RpcResponse, SDK_VERSION, SETTINGS_FRAME_VERSION, SdkVersion, SettingKey,
    SettingValues, ShellFrame, StatusKind, ThemeTokens, ToastTone, ViewedFile,
};
pub use error::{Error, Result};
pub use identity::{
    CommandId, ExtensionId, HostId, PaneId, PaneKind, PaneOrigin, PaneTypeId, SpaceId,
};
pub use port::{
    DAEMON_UNAVAILABLE, DaemonRequest, DaemonResponse, DaemonTransport, HOST_IDENTITY_CHANGED,
    HOST_UNREACHABLE, HttpMethod, NO_CREDENTIAL, PROXY_HEADER, RelayEndpoint, RelayScope,
    RequestTarget, Responder, UNKNOWN_HOST,
};
pub use protocol::{MAX_FRAME_BYTES, decode_pane_frame, decode_shell_frame, encode};
