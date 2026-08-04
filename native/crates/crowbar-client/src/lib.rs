#![forbid(unsafe_code)]

//! `crowbar-client` — the only thing in the tree that talks to the daemon.
//!
//! Owns the unix-socket HTTP client, and will own the WebSocket connection,
//! reconnect and backoff (spec §9.1). No domain logic lives here; it belongs in
//! `crowbar-core`.
//!
//! Dependency contract (§4.2): `crowbar-proto`, plus the §10.1 transport crates.
//!
//! **The socket path lives here, not in `crowbar-core`.** §4.2 gives
//! `crowbar-core` an edge to this crate and gives this crate `proto` only, so a
//! derivation placed in `core` would be one the transport itself could not
//! reach — the crate whose whole job is to dial the socket would have to be
//! told where it is by a layer above it. Deriving it here also keeps the
//! bytewise agreement with the Go daemon inside the crate that pays for getting
//! it wrong. It is still a pure function with no I/O ([`socket::Location`]), so
//! it sits under the same ≥98% line gate either way.
//!
//! Item 0.4 scope: one `GET /v0/health`. The rest of §9.1 arrives with a caller
//! that needs it — [`transport::get`] is S0.3's: `crowbar-sidecar`'s watchdog
//! needs a deep-readiness probe and a goroutine-dump capture, neither of which
//! is `/v0/health`'s fixed JSON envelope.

pub mod health;
pub mod socket;
pub mod transport;

pub use health::{Health, HealthError, fetch_health, fetch_health_with_timeout};
pub use socket::{Location, NoHome, RawEnv, fnv1a64};
pub use transport::{RawResponse, TransportError, get};

use std::fmt;
use std::path::PathBuf;

/// A resolved daemon endpoint plus whatever the last probe of it said.
///
/// Exists so a consumer gets the socket path *and* the outcome in one value:
/// when a probe fails, the path is the single most useful thing to show, and
/// re-deriving it at the call site would mean re-implementing this module.
#[derive(Debug)]
pub struct Probe {
    /// The socket this probe dialled.
    pub socket: PathBuf,
    /// What the daemon said, or why it did not.
    pub result: Result<Health, HealthError>,
}

/// Errors that stop a probe before a socket is even dialled.
#[derive(Debug)]
pub enum ProbeError {
    /// The environment does not name a Crowbar home.
    NoHome(NoHome),
}

impl fmt::Display for ProbeError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::NoHome(err) => write!(f, "{err}"),
        }
    }
}

impl std::error::Error for ProbeError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        match self {
            Self::NoHome(err) => Some(err),
        }
    }
}

impl From<NoHome> for ProbeError {
    fn from(err: NoHome) -> Self {
        Self::NoHome(err)
    }
}

/// Derive the daemon socket from the environment and ask it how it is.
///
/// Blocking, and the whole of what item 0.4 needs: derive, dial, report. A
/// down daemon is a [`Probe`] carrying an `Err`, not an error from this
/// function — "the daemon is not running" is a state the UI displays, not a
/// failure to probe.
///
/// # Errors
///
/// [`ProbeError`] only when the environment names no home at all, so there was
/// nothing to dial.
pub fn probe_daemon() -> Result<Probe, ProbeError> {
    let env = RawEnv::from_env();
    let socket = env.location().socket_path()?;
    let result = fetch_health(&socket);
    Ok(Probe { socket, result })
}

#[cfg(test)]
mod tests {
    use super::*;

    /// The probe reports a down daemon through `Probe.result`, not by failing.
    /// Any machine running this names either `CROWBAR_HOME` or `HOME`, so the
    /// derivation half succeeds; whether a daemon is behind the socket is
    /// deliberately not asserted — that is the live check in `crowbar-app`, and
    /// asserting it here would make the suite depend on a running daemon.
    #[test]
    fn probing_resolves_a_socket_even_when_the_daemon_is_down() {
        let probe = probe_daemon().expect("HOME or CROWBAR_HOME is set");
        let expected = RawEnv::from_env()
            .location()
            .socket_path()
            .expect("the same environment");

        assert_eq!(probe.socket, expected);
        assert!(!format!("{probe:?}").is_empty());
    }

    #[test]
    fn a_missing_home_surfaces_as_a_probe_error() {
        let err = ProbeError::from(NoHome);

        assert!(matches!(err, ProbeError::NoHome(NoHome)));
        assert_eq!(err.to_string(), NoHome.to_string());
        assert!(std::error::Error::source(&err).is_some());
        assert!(!format!("{err:?}").is_empty());
    }
}
