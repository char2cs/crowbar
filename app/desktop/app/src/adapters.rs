//! The operating system behind the shell's ports.

pub(crate) mod disk_files;
pub(crate) mod disk_preferences;
#[cfg(target_os = "linux")]
pub(crate) mod drop_relay;
#[cfg(target_os = "linux")]
pub(crate) mod key_places;
pub(crate) mod system_clock;
#[cfg(target_os = "linux")]
pub(crate) mod window_activity;
