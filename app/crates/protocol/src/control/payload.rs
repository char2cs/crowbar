//! Bounded values the frames carry: pane state, titles, params, display text, URLs, paths, statuses and toasts.

mod breakpoint;
mod extension_status;
mod paired_host;
mod pane_open;
mod pane_state;
mod pane_toast;
mod params;
mod text;
mod viewed_file;

pub use breakpoint::Breakpoint;
pub use extension_status::ExtensionStatus;
pub use extension_status::status_kind::StatusKind;
pub use paired_host::PairedHost;
pub use pane_open::PaneOpen;
pub use pane_open::open_mode::OpenMode;
pub use pane_state::{MAX_STATE_BYTES, PaneState};
pub use pane_toast::PaneToast;
pub use pane_toast::toast_tone::ToastTone;
pub use params::{MAX_PARAMS, Params};
pub use text::display_text::{DisplayText, MAX_DISPLAY_TEXT_BYTES};
pub use text::document_path::DocumentPath;
pub use text::external_url::ExternalUrl;
pub use text::fs_path::FsPath;
pub use text::pane_title::{MAX_TITLE_BYTES, PaneTitle};
pub use viewed_file::ViewedFile;
