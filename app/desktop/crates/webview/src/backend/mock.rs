//! An in-memory backend that records every call, for tests in this and other crates.

mod call;
mod desktop;
mod mock_platform;
mod views;

pub use call::Call;
pub use mock_platform::MockPlatform;
