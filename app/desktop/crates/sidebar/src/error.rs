use std::path::PathBuf;

#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    #[error("sidebar: command refused: {reason}")]
    Refused { reason: String },
    #[error("sidebar: {title}: {reason}")]
    Failed { title: String, reason: String },
    #[error("sidebar: read preferences {path}: {source}")]
    ReadPreferences {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("sidebar: write preferences {path}: {source}")]
    WritePreferences {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("sidebar: parse preferences {path}: {source}")]
    ParsePreferences {
        path: PathBuf,
        #[source]
        source: serde_json::Error,
    },
}
