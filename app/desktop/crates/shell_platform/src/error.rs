use std::path::PathBuf;

/// Why a desktop service could not run.
#[derive(Debug, thiserror::Error)]
#[non_exhaustive]
pub enum Error {
    /// The target is not something the shell may hand to the system.
    #[error("refusing to open {target:?}: {reason}")]
    Refused {
        /// What was asked for.
        target: String,
        /// Why it was refused.
        reason: &'static str,
    },
    /// The path to reveal does not exist or cannot be resolved.
    #[error("cannot resolve {path}: {source}")]
    Unresolved {
        /// The path.
        path: PathBuf,
        /// The failure.
        #[source]
        source: std::io::Error,
    },
    /// The desktop's launcher could not be started.
    #[error("cannot start {program}: {source}")]
    Spawn {
        /// The launcher program.
        program: &'static str,
        /// The failure.
        #[source]
        source: std::io::Error,
    },
}
