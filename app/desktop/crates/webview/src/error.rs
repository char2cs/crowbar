use raw_window_handle::HandleError;
use thiserror::Error;

use crate::web_view_id::WebViewId;

/// Everything that can go wrong while hosting web views.
#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    /// This platform cannot host web views.
    #[error("web views are not supported on this platform")]
    UnsupportedPlatform,
    /// The parent window's handle is of a kind this platform cannot embed into.
    #[error("unsupported parent window handle: {0}")]
    UnsupportedParent(&'static str),
    /// The parent window did not give out a handle.
    #[error("parent window handle unavailable: {0}")]
    Handle(#[from] HandleError),
    /// The toolkit thread could not be started.
    #[error("platform thread failed to start: {0}")]
    PlatformThread(String),
    /// The toolkit thread has stopped.
    #[error("platform thread has stopped")]
    HostStopped,
    /// A call that must run on the main thread ran elsewhere.
    #[error("must be called on the main thread")]
    NotMainThread,
    /// No live web view has this id.
    #[error("no web view {0}")]
    UnknownWebView(WebViewId),
    /// The engine refused an operation.
    #[error("web view engine error: {0}")]
    Engine(#[from] wry::Error),
}

/// Result alias using the crate [`enum@Error`].
pub type Result<T> = std::result::Result<T, Error>;
