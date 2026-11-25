//! The GTK backend: web views on the GTK thread, their pages, placement and capture.

#[cfg(target_os = "linux")]
mod alert_asking;
#[cfg(target_os = "linux")]
mod bounded_wait;
mod capture;
#[cfg(target_os = "linux")]
mod create_reply;
#[cfg(target_os = "linux")]
mod desktop_settings;
#[cfg(target_os = "linux")]
mod file_choosing;
#[cfg(target_os = "linux")]
mod gtk_platform;
#[cfg(target_os = "linux")]
mod gtk_state;
#[cfg(target_os = "linux")]
mod menu_popping;
#[cfg(target_os = "linux")]
mod page;
#[cfg(target_os = "linux")]
mod views;

#[cfg(target_os = "linux")]
pub(crate) use gtk_platform::GtkPlatform;
