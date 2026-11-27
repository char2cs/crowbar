#[cfg(feature = "mock")]
mod alerts;
#[cfg(feature = "mock")]
mod call;
#[cfg(feature = "mock")]
mod choosing;
#[cfg(feature = "mock")]
mod menus;
#[cfg(feature = "mock")]
mod mock_platform;
#[cfg(feature = "mock")]
mod page;
#[cfg(feature = "mock")]
mod views;

#[cfg(feature = "mock")]
pub use call::Call;
#[cfg(feature = "mock")]
pub use mock_platform::MockPlatform;
