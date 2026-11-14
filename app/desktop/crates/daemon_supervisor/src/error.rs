use std::path::PathBuf;

use thiserror::Error;

/// Everything that can stop the daemon from being found, started, kept up or stopped.
#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    /// No `crowbar-api` binary was found.
    #[error("cannot find the crowbar-api binary (looked in {looked})")]
    BinaryNotFound {
        /// The places that were searched.
        looked: String,
    },
    /// The daemon binary could not be run.
    #[error("cannot run {path}: {source}")]
    Spawn {
        /// The binary that was run.
        path: PathBuf,
        /// The OS error.
        #[source]
        source: std::io::Error,
    },
    /// `crowbar-api paths` failed or printed nothing usable.
    #[error("cannot resolve the Crowbar home: {0}")]
    Paths(String),
    /// The daemon halted on purpose, and no restart fixes it.
    #[error("the daemon halted: {0}")]
    Halted(String),
    /// The daemon never became healthy.
    #[error("daemon did not become healthy in {0} attempts")]
    GaveUp(u32),
    /// The daemon exited before it became healthy.
    #[error("the daemon exited while starting ({0})")]
    ExitedEarly(String),
    /// A restart was asked for while the daemon is running.
    #[error("the backend is already running")]
    AlreadyRunning,
    /// The supervisor has been stopped.
    #[error("the supervisor is stopped")]
    Stopped,
    /// The daemon log could not be used.
    #[error("daemon log {path}: {source}")]
    Log {
        /// The log file.
        path: PathBuf,
        /// The OS error.
        #[source]
        source: std::io::Error,
    },
}
