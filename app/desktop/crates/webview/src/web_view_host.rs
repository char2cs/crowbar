use std::cell::Cell;
use std::rc::Rc;

use raw_window_handle::HasWindowHandle;

use crate::error::Result;
use crate::host_config::HostConfig;
use crate::parent_window::ParentWindow;
use crate::platform::Platform;
use crate::web_view_config::WebViewConfig;
use crate::web_view_handle::WebViewHandle;
use crate::web_view_id::WebViewId;

/// Creates web views as children of a window and owns the platform thread they live on.
pub struct WebViewHost {
    platform: Rc<dyn Platform>,
    next_id: Cell<u64>,
}

impl std::fmt::Debug for WebViewHost {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("WebViewHost").finish_non_exhaustive()
    }
}

impl WebViewHost {
    /// Starts the backend for the current platform. Call it on the main thread.
    ///
    /// # Errors
    /// [`Error::UnsupportedPlatform`](crate::Error::UnsupportedPlatform), or the error of the
    /// backend failing to start.
    pub fn native(config: HostConfig) -> Result<Self> {
        Ok(Self::with_platform(start_native(config)?))
    }

    /// Builds a host on an explicit backend, for example the recording one used in tests.
    #[must_use]
    pub fn with_platform(platform: Rc<dyn Platform>) -> Self {
        Self {
            platform,
            next_id: Cell::new(0),
        }
    }

    /// Creates a web view inside `window` and returns its handle.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the window cannot host web views or the engine fails.
    pub fn create(
        &self,
        window: &impl HasWindowHandle,
        config: WebViewConfig,
    ) -> Result<WebViewHandle> {
        let parent = ParentWindow::from_window(window)?;
        self.create_in(parent, config)
    }

    /// Like [`WebViewHost::create`], for a parent that is already extracted.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the engine fails.
    pub fn create_in(&self, parent: ParentWindow, config: WebViewConfig) -> Result<WebViewHandle> {
        let id = WebViewId(self.next_id.get());
        self.next_id.set(id.0 + 1);
        self.platform.create(id, parent, config)?;
        Ok(WebViewHandle::new(Rc::clone(&self.platform), id))
    }
}

#[cfg(target_os = "linux")]
fn start_native(config: HostConfig) -> Result<Rc<dyn Platform>> {
    Ok(Rc::new(crate::linux_platform::LinuxPlatform::start(
        config,
    )?))
}

#[cfg(target_os = "macos")]
fn start_native(config: HostConfig) -> Result<Rc<dyn Platform>> {
    Ok(Rc::new(crate::macos_platform::MacosPlatform::start(
        config,
    )?))
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
fn start_native(_config: HostConfig) -> Result<Rc<dyn Platform>> {
    Err(crate::error::Error::UnsupportedPlatform)
}

#[cfg(test)]
#[path = "web_view_host_tests.rs"]
mod tests;
