//! Desktop services of the Crowbar shell, and the one home of its `unsafe` code.
//!
//! [`follow_link`] decides where a link may go (stay in the app, leave for the system browser or
//! be refused). [`open_external`] and [`reveal_in_file_manager`] hand a link or a path to the
//! desktop's own tools. The command lines are built by pure functions so they are tested without
//! a desktop; only starting the launcher touches the system.
//!
//! The OS layer sits in `linux/` and `macos/`: the native parent windows web views are embedded
//! into (`XlibParent`, `AppkitParent`), the X keyboard focus (`InputFocus`), the X keyboard mapping (`KeyMap`) and the X11 session
//! switch (`force_x11`). Every `unsafe` block of the desktop app lives there, behind a safe type
//! or function (see the README).

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
pub use linux::{FocusChange, InputFocus, KeyMap, UsKey, XlibParent, force_x11};
#[cfg(target_os = "macos")]
pub use macos::AppkitParent;
pub use platform::{follow_link, open_external, open_path, reveal_in_file_manager};
