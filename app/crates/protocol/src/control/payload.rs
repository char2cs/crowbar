//! Bounded values the frames carry: pane state, titles, params, display text, URLs, paths, statuses and toasts.

mod byte_counter;
mod display_text;
mod extension_status;
mod external_url;
mod fs_path;
mod pane_state;
mod pane_title;
mod pane_toast;
mod params;
mod status_kind;
mod toast_tone;
mod viewed_file;

pub use display_text::{DisplayText, MAX_DISPLAY_TEXT_BYTES};
pub use extension_status::ExtensionStatus;
pub use external_url::ExternalUrl;
pub use fs_path::FsPath;
pub use pane_state::{MAX_STATE_BYTES, PaneState};
pub use pane_title::{MAX_TITLE_BYTES, PaneTitle};
pub use pane_toast::PaneToast;
pub use params::{MAX_PARAMS, Params};
pub use status_kind::StatusKind;
pub use toast_tone::ToastTone;
pub use viewed_file::ViewedFile;
