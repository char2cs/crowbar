use protocol::PaneTypeId;
use thiserror::Error;

#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    #[error("pane type {0} is already registered")]
    DuplicateType(PaneTypeId),
    #[error("invalid frame from pane: {0}")]
    Frame(#[from] protocol::Error),
    #[error("pane {0} is not shown")]
    NotShown(protocol::PaneId),
    #[error("web view error: {0}")]
    WebView(#[from] webview::Error),
}

pub type Result<T> = std::result::Result<T, Error>;
