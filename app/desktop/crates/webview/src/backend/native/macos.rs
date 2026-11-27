#[cfg(target_os = "macos")]
mod alert_asking;
#[cfg(target_os = "macos")]
mod appkit_platform;
#[cfg(target_os = "macos")]
mod browsing_data;
#[cfg(target_os = "macos")]
mod data_store;
#[cfg(target_os = "macos")]
mod desktop_settings;
#[cfg(target_os = "macos")]
mod file_choosing;
#[cfg(target_os = "macos")]
mod lifecycle;
#[cfg(target_os = "macos")]
mod menu_popping;
#[cfg(target_os = "macos")]
mod page;

#[cfg(target_os = "macos")]
pub(crate) use appkit_platform::AppkitPlatform;
