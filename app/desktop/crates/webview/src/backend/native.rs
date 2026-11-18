//! The native backends of each OS and the wry builder steps they share.

#[cfg(target_os = "linux")]
mod linux;
#[cfg(target_os = "macos")]
mod macos;
mod wry_builder;

#[cfg(target_os = "linux")]
pub(crate) use linux::GtkPlatform;
#[cfg(target_os = "macos")]
pub(crate) use macos::AppkitPlatform;
