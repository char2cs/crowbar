//! The GTK backend: web views on the GTK thread, their pages, placement and capture.

mod alert_asking;
mod capture;
mod create_reply;
mod desktop_settings;
mod file_choosing;
mod gtk_platform;
mod gtk_state;
mod menu_popping;
mod page;
#[cfg(test)]
mod test_support;
mod views;

pub(crate) use gtk_platform::GtkPlatform;
