use std::path::PathBuf;

#[derive(Debug, thiserror::Error)]
pub(crate) enum Error {
    #[error("reading {0}: {1}")]
    Read(PathBuf, #[source] std::io::Error),
    #[error("writing {0}: {1}")]
    Write(PathBuf, #[source] std::io::Error),
    #[error("{0} is not a valid layout document: {1}")]
    Parse(PathBuf, #[source] serde_json::Error),
    #[error("the pane pages are missing: {0} does not exist (set CROWBAR_PANES_DIR)")]
    PanesMissing(PathBuf),
    #[error("starting the layout writer: {0}")]
    LayoutWriter(#[source] std::io::Error),
    #[error("starting the daemon link: {0}")]
    Daemon(String),
    #[error(transparent)]
    WebView(#[from] webview::Error),
    #[error("the settings modal could not be built: {0}")]
    Settings(String),
    #[error("the layout refused a command: {0}")]
    Layout(#[from] layout::Error),
    #[error("a pane host call failed: {0}")]
    Host(#[from] pane_host::Error),
    #[error("a protocol value was invalid: {0}")]
    Protocol(#[from] protocol::Error),
}

pub(crate) type Result<T> = std::result::Result<T, Error>;
