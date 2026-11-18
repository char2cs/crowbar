//! The AppKit backend: WKWebView views and their website data.

#[cfg(target_os = "macos")]
mod appkit_platform;
#[cfg(target_os = "macos")]
mod browsing_data;
#[cfg(target_os = "macos")]
mod data_store;

#[cfg(target_os = "macos")]
pub(crate) use appkit_platform::AppkitPlatform;
