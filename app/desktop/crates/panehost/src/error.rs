use protocol::PaneTypeId;
use thiserror::Error;

/// Everything that can go wrong in the pane host.
#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    /// A pane type with this id is already registered.
    #[error("pane type {0} is already registered")]
    DuplicateType(PaneTypeId),
    /// A page sent something that is not a valid control frame.
    #[error("invalid frame from pane: {0}")]
    Frame(#[from] protocol::Error),
    /// The pane has no web view to talk to.
    #[error("pane {0} is not shown")]
    NotShown(protocol::PaneId),
    /// The web view layer failed.
    #[error("web view error: {0}")]
    WebView(#[from] webview::Error),
}

/// Result alias using the crate [`enum@Error`].
pub type Result<T> = std::result::Result<T, Error>;
