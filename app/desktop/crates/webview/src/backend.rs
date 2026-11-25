//! The Platform trait a backend implements, its capabilities and the host configuration it reads.

mod alert_asking;
mod browsing_data;
mod caret_blink;
mod desktop_settings;
mod file_choosing;
mod host_config;
mod menu_popping;
#[cfg(feature = "mock")]
mod mock;
mod native;
mod page_control;
mod platform;
mod view_capture;
mod view_focus;
mod view_layout;
mod view_lifecycle;

pub use alert_asking::{AlertAsking, ChosenButton};
pub use browsing_data::BrowsingData;
pub use caret_blink::{CaretBlink, CaretReport};
pub use desktop_settings::DesktopSettings;
pub use file_choosing::{ChosenFiles, FileChoosing};
pub use host_config::HostConfig;
pub use menu_popping::{ChosenEntry, MenuPopping};
#[cfg(feature = "mock")]
pub use mock::{Call, MockPlatform};
#[cfg(target_os = "macos")]
pub(crate) use native::AppkitPlatform;
#[cfg(target_os = "linux")]
pub(crate) use native::GtkPlatform;
pub use page_control::PageControl;
pub use platform::Platform;
pub use view_capture::ViewCapture;
pub use view_focus::ViewFocus;
pub use view_layout::ViewLayout;
pub use view_lifecycle::ViewLifecycle;
