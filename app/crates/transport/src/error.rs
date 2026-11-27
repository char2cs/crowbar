use std::io;
use std::path::PathBuf;

use protocol::HostId;
use thiserror::Error;

use crate::response::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, NO_CREDENTIAL, UNKNOWN_HOST};

type Cause = Box<dyn std::error::Error + Send + Sync>;

#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    #[error("host id {id} cannot name a remote host: {why}")]
    BadHostId { id: HostId, why: &'static str },
    #[error(transparent)]
    Id(#[from] protocol::Error),
    #[error("{text:?} is not a host:port address: {why}")]
    MalformedEndpoint { text: String, why: &'static str },
    #[error("cannot resolve {host}: {reason}")]
    Unresolvable { host: String, reason: String },
    #[error("cannot reach {endpoint}: {source}")]
    Unreachable {
        endpoint: String,
        #[source]
        source: io::Error,
    },
    #[error("this origin may not reach {0:?}")]
    Forbidden(String),
    #[error("hosts file {path}: {source}")]
    HostsIo {
        path: PathBuf,
        #[source]
        source: io::Error,
    },
    #[error("hosts file {path} is corrupt: {source}")]
    HostsCorrupt {
        path: PathBuf,
        #[source]
        source: serde_json::Error,
    },
    #[error("hosts file {path} holds a bad entry: {why}")]
    HostsInvalid { path: PathBuf, why: String },
    #[error("hosts file {path} has version {found}, this Crowbar reads version {supported}")]
    HostsVersion {
        path: PathBuf,
        found: u32,
        supported: u32,
    },
    #[error("{0}")]
    Store(String),
    #[error("{0}")]
    HostUnreachable(String),
    #[error("{0}")]
    IdentityChanged(String),
    #[error("http exchange failed: {0}")]
    Exchange(#[source] Cause),
    #[error("bad request: {0}")]
    BadRequest(#[source] Cause),
    #[error("invalid request: {part}: {source}")]
    Malformed {
        part: &'static str,
        #[source]
        source: Cause,
    },
    #[error("response too large")]
    TooLarge,
    #[error("reading the response: {0}")]
    Body(String),
    #[error("unknown remote host {0}")]
    UnknownHost(HostId),
    #[error("no stored credential for this host \u{2014} pair it again")]
    NoCredential,
    #[error("the stored credential is not a valid header value: {0}")]
    BadToken(#[source] http::header::InvalidHeaderValue),
    #[error("blocking task failed: {0}")]
    BlockingTask(String),
    #[error("{endpoint} did not answer within the probe deadline")]
    ProbeTimeout {
        endpoint: String,
        #[source]
        deadline: tokio::time::error::Elapsed,
    },
    #[error("TLS handshake with {endpoint} failed: {reason}")]
    Handshake { endpoint: String, reason: String },
    #[error("{0} presented no certificate")]
    NoCertificate(String),
    #[error("{endpoint} did not finish pairing in time")]
    PairTimeout {
        endpoint: String,
        #[source]
        deadline: tokio::time::error::Elapsed,
    },
    #[error("pairing with {endpoint} failed: {source}")]
    PairExchange {
        endpoint: String,
        #[source]
        source: Box<Error>,
    },
    #[error("the host refused ({status}): {reason}")]
    Refused { status: u16, reason: String },
    #[error("the host's pairing answer carried no {0}")]
    MissingField(&'static str),
    #[error("pairing with {endpoint} could not be recorded ({stage}: {cause}); {cleanup}")]
    NotRecorded {
        endpoint: String,
        stage: &'static str,
        cause: String,
        cleanup: String,
    },
    #[error("removed host {host}, but its stored credential remains: {source}")]
    CredentialRemains {
        host: HostId,
        #[source]
        source: Box<Error>,
    },
    #[error("the local daemon has not published a socket")]
    NoSocket,
    #[error("cannot connect to the local daemon socket: {0}")]
    LocalSocket(#[source] io::Error),
    #[error("the local daemon socket is not ours: {0}")]
    UntrustedSocket(String),
    #[error("the daemon did not answer the upgrade in time")]
    UpgradeTimeout,
    #[error("websocket: {0}")]
    WebSocket(#[source] Cause),
    #[error("relay cannot bind 127.0.0.1: {0}")]
    Bind(#[source] io::Error),
    #[error("invalid request: {0}")]
    InvalidRequest(&'static str),
    #[error("random source: {0}")]
    Random(String),
}

impl Error {
    pub(crate) fn exchange(source: impl std::error::Error + Send + Sync + 'static) -> Self {
        Self::Exchange(Box::new(source))
    }

    pub(crate) fn bad_request(source: impl std::error::Error + Send + Sync + 'static) -> Self {
        Self::BadRequest(Box::new(source))
    }

    pub(crate) fn malformed(
        part: &'static str,
        source: impl std::error::Error + Send + Sync + 'static,
    ) -> Self {
        Self::Malformed {
            part,
            source: Box::new(source),
        }
    }

    pub(crate) fn websocket(source: impl std::error::Error + Send + Sync + 'static) -> Self {
        Self::WebSocket(Box::new(source))
    }

    pub(crate) fn marker(&self) -> Option<&'static str> {
        match self {
            Self::HostUnreachable(_) | Self::UpgradeTimeout => Some(HOST_UNREACHABLE),
            Self::IdentityChanged(_) => Some(HOST_IDENTITY_CHANGED),
            Self::NoCredential => Some(NO_CREDENTIAL),
            Self::UnknownHost(_) => Some(UNKNOWN_HOST),
            _ => None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::response::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, NO_CREDENTIAL, UNKNOWN_HOST};

    #[test]
    fn a_page_gets_a_marker_for_failures_it_can_act_on() {
        let host = HostId::new("host-1").unwrap();
        for (error, marker) in [
            (
                Error::IdentityChanged("x".into()),
                Some(HOST_IDENTITY_CHANGED),
            ),
            (Error::HostUnreachable("x".into()), Some(HOST_UNREACHABLE)),
            (Error::UpgradeTimeout, Some(HOST_UNREACHABLE)),
            (Error::NoCredential, Some(NO_CREDENTIAL)),
            (Error::UnknownHost(host), Some(UNKNOWN_HOST)),
            (Error::Forbidden("x".into()), None),
            (Error::TooLarge, None),
        ] {
            assert_eq!(error.marker(), marker, "{error}");
        }
    }

    #[test]
    fn local_socket_errors_never_print_a_path() {
        let error = Error::LocalSocket(io::Error::new(io::ErrorKind::NotFound, "no such file"));
        assert!(!error.to_string().contains('/'), "{error}");
        assert!(
            std::error::Error::source(&error).is_some(),
            "the io error stays the source"
        );
    }
}
