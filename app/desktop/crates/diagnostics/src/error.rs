use std::path::PathBuf;

#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error("cannot open the log file in {dir}: {source}")]
    LogFile {
        dir: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("cannot write the diagnostics bundle {path}: {source}")]
    Bundle {
        path: PathBuf,
        #[source]
        source: Box<dyn std::error::Error + Send + Sync>,
    },
    #[error("a logging subscriber is already installed")]
    AlreadyInstalled(#[source] Box<dyn std::error::Error + Send + Sync>),
}
