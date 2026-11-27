#[derive(Debug, thiserror::Error, PartialEq, Eq)]
#[non_exhaustive]
pub enum Error {
    #[error("not a valid URL: {text:?}")]
    Invalid {
        text: String,
        #[source]
        source: url::ParseError,
    },
    #[error("browser panes load only http, https and about:blank, not {0:?}")]
    Scheme(String),
}
