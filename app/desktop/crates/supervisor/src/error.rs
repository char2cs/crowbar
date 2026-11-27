use std::path::PathBuf;

use rustix::io::Errno;
use thiserror::Error;

#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    #[error("cannot find the crowbar-api binary (looked in {looked})")]
    BinaryNotFound { looked: String },
    #[error("cannot run {path}: {source}")]
    Spawn {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("cannot resolve the Crowbar home: {0}")]
    Paths(String),
    #[error("`crowbar-api paths` did not answer in time")]
    PathsTimeout(#[source] tokio::time::error::Elapsed),
    #[error("the daemon halted: {message}")]
    Halted {
        reason: Option<String>,
        message: String,
    },
    #[error("daemon did not become healthy in {0} attempts")]
    GaveUp(u32),
    #[error("the daemon exited while starting ({0})")]
    ExitedEarly(String),
    #[error("the backend is already running")]
    AlreadyRunning,
    #[error("the supervisor is stopped")]
    Stopped,
    #[error("daemon log {path}: {source}")]
    Log {
        path: PathBuf,
        #[source]
        source: std::io::Error,
    },
    #[error("cannot dial the daemon: {source}")]
    Dial {
        #[source]
        source: std::io::Error,
    },
    #[error("request {target} failed: {source}")]
    Exchange {
        target: String,
        #[source]
        source: Box<dyn std::error::Error + Send + Sync>,
    },
    #[error("{target} returned status {status}")]
    Status { target: String, status: u16 },
    #[error("{target} returned more than {limit} bytes")]
    BodyTooLarge { target: String, limit: usize },
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
