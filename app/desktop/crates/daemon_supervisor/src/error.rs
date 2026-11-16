use std::path::PathBuf;

use rustix::io::Errno;
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
    /// `crowbar-api paths` did not answer in time.
    #[error("`crowbar-api paths` did not answer in time")]
    PathsTimeout(#[source] tokio::time::error::Elapsed),
    /// The daemon halted on purpose, and no restart fixes it.
    #[error("the daemon halted: {message}")]
    Halted {
        /// The machine-readable halt reason, when the daemon gave one.
        reason: Option<String>,
        /// What the daemon said.
        message: String,
    },
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
    /// The daemon's socket could not be dialled.
    #[error("cannot dial the daemon: {source}")]
    Dial {
        /// The OS error, kept so a full descriptor table can be told from a dead daemon.
        #[source]
        source: std::io::Error,
    },
    /// A request to the daemon failed after the connection was made.
    #[error("request {target} failed: {source}")]
    Exchange {
        /// The request path.
        target: String,
        /// What went wrong.
        #[source]
        source: Box<dyn std::error::Error + Send + Sync>,
    },
    /// The daemon answered with a status other than success.
    #[error("{target} returned status {status}")]
    Status {
        /// The request path.
        target: String,
        /// The HTTP status code.
        status: u16,
    },
    /// The daemon's answer was larger than the reader accepts.
    #[error("{target} returned more than {limit} bytes")]
    BodyTooLarge {
        /// The request path.
        target: String,
        /// The limit that was exceeded.
        limit: usize,
    },
}

impl Error {
    #[must_use]
    pub(crate) fn is_descriptor_exhaustion(&self) -> bool {
        let chain =
            std::iter::successors(Some(self as &(dyn std::error::Error + 'static)), |error| {
                error.source()
            });
        chain.into_iter().any(is_out_of_descriptors)
    }
}

fn is_out_of_descriptors(error: &(dyn std::error::Error + 'static)) -> bool {
    error
        .downcast_ref::<std::io::Error>()
        .and_then(std::io::Error::raw_os_error)
        .is_some_and(|raw| raw == Errno::MFILE.raw_os_error() || raw == Errno::NFILE.raw_os_error())
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;

    use super::*;

    fn os(errno: Errno) -> std::io::Error {
        std::io::Error::from_raw_os_error(errno.raw_os_error())
    }

    #[test]
    fn running_out_of_descriptors_is_recognised_wherever_it_sits_in_the_chain() {
        let direct = Error::Dial {
            source: os(Errno::MFILE),
        };
        let system_wide = Error::Dial {
            source: os(Errno::NFILE),
        };
        let nested = Error::Exchange {
            target: "/v0/health".into(),
            source: Box::new(os(Errno::MFILE)),
        };
        for error in [direct, system_wide, nested] {
            assert!(error.is_descriptor_exhaustion(), "{error:?}");
        }
    }

    #[test]
    fn other_failures_are_not_descriptor_exhaustion() {
        let cases = [
            Error::Dial {
                source: os(Errno::CONNREFUSED),
            },
            Error::Dial {
                source: std::io::Error::from(ErrorKind::NotFound),
            },
            Error::Status {
                target: "/x".into(),
                status: 503,
            },
            Error::GaveUp(3),
        ];
        for error in cases {
            assert!(!error.is_descriptor_exhaustion(), "{error:?}");
        }
    }
}
