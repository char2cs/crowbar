mod desktop;
mod host_config;
#[cfg(feature = "mock")]
mod mock;
mod native;
mod platform;
mod view;

pub use desktop::alert_asking::{AlertAsking, ChosenButton};
pub use desktop::caret_blink::{CaretBlink, CaretReport};
pub use desktop::color_scheme::ColorScheme;
pub use desktop::desktop_settings::DesktopSettings;
pub use desktop::file_choosing::{ChosenFiles, FileChoosing};
pub use desktop::menu_popping::{ChosenEntry, MenuPopping};
pub use host_config::HostConfig;
#[cfg(feature = "mock")]
pub use mock::{Call, MockPlatform};
#[cfg(target_os = "macos")]
pub(crate) use native::AppkitPlatform;
#[cfg(target_os = "linux")]
pub(crate) use native::GtkPlatform;
pub use platform::Platform;
pub use view::browsing_data::BrowsingData;
pub use view::page_control::PageControl;
pub use view::view_capture::ViewCapture;
pub use view::view_cutout::ViewCutout;
pub use view::view_focus::ViewFocus;
pub use view::view_layout::ViewLayout;
pub use view::view_lifecycle::ViewLifecycle;
