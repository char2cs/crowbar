//! Desktop services of the Crowbar shell.
//!
//! [`classify`] decides where a link may go (stay in the app, leave for the system browser or
//! be refused). [`open_external`] and [`reveal_in_file_manager`] hand a link or a path to the
//! desktop's own tools. The command lines are built by pure functions ([`Launch`]) so they are
//! tested without a desktop; only [`Launch::spawn`] touches the system.

mod desktop;
mod error;
mod launch;
mod link_target;
mod shell_platform;

pub use desktop::Desktop;
pub use error::Error;
pub use launch::Launch;
pub use link_target::{LinkTarget, classify};
pub use shell_platform::{follow_link, open_external, open_path, reveal_in_file_manager};
