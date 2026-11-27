mod desktop;
mod error;
mod launch;
mod link_target;
#[cfg(target_os = "linux")]
mod linux;
#[cfg(target_os = "macos")]
mod macos;
mod platform;

pub use error::Error;
#[cfg(target_os = "linux")]
pub use linux::{
    Card, FloatPlacer, FocusChange, InputFocus, KeyMap, PageCut, PageShape, Rounded, UsKey,
    XdndProxy, XlibParent, force_x11,
};
#[cfg(target_os = "macos")]
pub use macos::{
    AppkitParent, ClipShape, ContextMenu, Frost, MenuRow, PageClip, frost_window, on_next_turn,
};
pub use platform::{follow_link, open_external, open_path, reveal_in_file_manager};
