mod web_view_handle;

pub use web_view_handle::{RECEIVER, WebViewHandle};

use std::cell::Cell;
use std::rc::Rc;

use raw_window_handle::HasWindowHandle;

use crate::backend::HostConfig;
use crate::backend::Platform;
use crate::error::Result;
use crate::view::ParentWindow;
use crate::view::Snapshot;
use crate::view::WebViewConfig;
use crate::view::WebViewId;

/// Creates web views as children of a window and owns the platform thread they live on.
pub struct WebViewHost {
    platform: Rc<dyn Platform>,
    next_id: Cell<u64>,
}

impl std::fmt::Debug for WebViewHost {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
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

    /// Deletes cookies, cache and site storage of every browser view.
    ///
    /// # Errors
    /// An [`Error`](crate::Error) when the backend has stopped or the engine refuses.
    pub fn clear_browsing_data(&self) -> Result<()> {
        self.platform.clear_browsing_data()
    }

    /// The pixels of several views, asked together under one deadline, in the order of `ids`.
    #[must_use]
    pub fn snapshot_all(
        &self,
        ids: &[WebViewId],
    ) -> Vec<Result<Option<Snapshot>>> {
        self.platform.snapshot_all(ids)
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
    pub fn create_in(
        &self,
        parent: ParentWindow,
        config: WebViewConfig,
    ) -> Result<WebViewHandle> {
        let id = WebViewId(self.next_id.get());
        self.next_id.set(id.0 + 1);
        self.platform.create(id, parent, config)?;
        Ok(WebViewHandle::new(Rc::clone(&self.platform), id))
    }
}

#[cfg(target_os = "linux")]
fn start_native(config: HostConfig) -> Result<Rc<dyn Platform>> {
    Ok(Rc::new(crate::backend::LinuxPlatform::start(config)?))
}

#[cfg(target_os = "macos")]
fn start_native(config: HostConfig) -> Result<Rc<dyn Platform>> {
    Ok(Rc::new(crate::backend::MacosPlatform::start(config)?))
}

#[cfg(not(any(target_os = "linux", target_os = "macos")))]
fn start_native(_config: HostConfig) -> Result<Rc<dyn Platform>> {
    Err(crate::error::Error::UnsupportedPlatform)
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::backend::Call;
    use crate::backend::MockPlatform;
    use crate::view::Bounds;
    use crate::view::Visibility;
    use crate::view::WebViewKind;

    fn rect(
        x: f64,
        y: f64,
        width: f64,
        height: f64,
    ) -> Bounds {
        Bounds {
            x,
            y,
            width,
            height,
        }
    }

    fn config(kind: WebViewKind) -> WebViewConfig {
        WebViewConfig {
            url: "crowbar://local/".into(),
            bounds: rect(1.0, 2.0, 3.0, 4.0),
            kind,
            visibility: Visibility::Hidden,
            background: None,
            initialization_script: None,
            on_ipc: Arc::new(|_| {}),
            on_page: None,
        }
    }

    #[test]
    fn every_view_gets_a_fresh_id() {
        let mock = Rc::new(MockPlatform::default());
        let host = WebViewHost::with_platform(Rc::<MockPlatform>::clone(&mock));
        let a = host
            .create_in(ParentWindow::X11(7), config(WebViewKind::Core))
            .unwrap();
        let b = host
            .create_in(ParentWindow::X11(7), config(WebViewKind::Core))
            .unwrap();
        assert_ne!(a.id(), b.id(), "ids differ");
        assert_eq!(mock.live_views().len(), 2, "both alive");
    }

    #[test]
    fn a_failed_create_registers_nothing() {
        let mock = Rc::new(MockPlatform::default());
        let host = WebViewHost::with_platform(Rc::<MockPlatform>::clone(&mock));
        mock.fail_next_create();
        assert!(
            host.create_in(ParentWindow::X11(7), config(WebViewKind::Core))
                .is_err(),
            "fails"
        );
        assert!(
            mock.calls().is_empty() && mock.live_views().is_empty(),
            "nothing recorded"
        );
    }

    #[test]
    fn the_background_of_the_config_is_what_the_view_is_created_with() {
        let mock = Rc::new(MockPlatform::default());
        let host = WebViewHost::with_platform(Rc::<MockPlatform>::clone(&mock));
        let color = crate::view::Color {
            red: 1,
            green: 2,
            blue: 3,
            alpha: 255,
        };
        let mut with_background = config(WebViewKind::Core);
        with_background.background = Some(color);
        host.create_in(ParentWindow::X11(7), with_background)
            .unwrap();
        assert!(
            matches!(
                mock.calls().first(),
                Some(Call::Create { background: Some(made), .. }) if *made == color
            ),
            "{:?}",
            mock.calls()
        );
    }

    #[test]
    fn clearing_browsing_data_reaches_the_backend() {
        let mock = Rc::new(MockPlatform::default());
        let host = WebViewHost::with_platform(Rc::<MockPlatform>::clone(&mock));
        assert!(host.clear_browsing_data().is_ok(), "accepted");
        assert_eq!(mock.calls(), [Call::ClearBrowsingData]);
    }

    #[test]
    fn the_config_reaches_the_backend_unchanged() {
        let git = protocol::ExtensionId::new("git").unwrap();
        let cases = [
            ("core", WebViewKind::Core),
            ("extension", WebViewKind::Extension(git)),
        ];
        for (name, kind) in cases {
            let mock = Rc::new(MockPlatform::default());
            let host = WebViewHost::with_platform(Rc::<MockPlatform>::clone(&mock));
            let _view = host
                .create_in(ParentWindow::X11(7), config(kind.clone()))
                .unwrap();
            assert!(
                matches!(
                    mock.calls().first(),
                    Some(Call::Create { parent: ParentWindow::X11(7), kind: made, visibility: Visibility::Hidden, .. }) if *made == kind
                ),
                "{name}"
            );
        }
    }
}
