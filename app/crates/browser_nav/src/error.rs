/// Why a URL cannot be loaded in a browser pane.
#[derive(Debug, thiserror::Error, PartialEq, Eq)]
#[non_exhaustive]
pub enum Error {
    /// The text is not a URL.
    #[error("not a valid URL: {0:?}")]
    Invalid(String),
    /// The URL has a scheme a browser pane does not load.
    #[error("browser panes load only http, https and about:blank, not {0:?}")]
    Scheme(String),
}
