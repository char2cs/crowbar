//! The sdk@1 control frames a pane and the shell exchange, with their envelope and lifecycle.

mod call;
mod config;
mod envelope;
mod lifecycle;
mod pane_frame;
mod payload;
mod sdk_version;
mod shell_frame;

pub use call::{ErrorCode, EventName, MethodName, RequestId, RpcError, RpcOutcome, RpcResponse};
pub use config::{
    KeyBinding, KeyChord, MAX_SETTING_VALUES, MAX_THEME_TOKENS, PANE_SETTING_KEYS, Precedence,
    SETTINGS_FRAME_VERSION, SettingKey, SettingValues, ThemeTokens,
};
pub(crate) use envelope::Envelope;
pub use lifecycle::Lifecycle;
pub use pane_frame::PaneFrame;
pub use payload::{
    DisplayText, ExtensionStatus, ExternalUrl, FsPath, MAX_DISPLAY_TEXT_BYTES, MAX_PARAMS,
    MAX_STATE_BYTES, MAX_TITLE_BYTES, PaneState, PaneTitle, PaneToast, Params, StatusKind,
    ToastTone, ViewedFile,
};
pub use sdk_version::{SDK_VERSION, SdkVersion};
pub use shell_frame::ShellFrame;
