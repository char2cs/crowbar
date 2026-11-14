use std::io;
use std::path::PathBuf;

use protocol::HostId;
use thiserror::Error;

use crate::endpoint::Endpoint;
use crate::response::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, NO_CREDENTIAL, UNKNOWN_HOST};

type Cause = Box<dyn std::error::Error + Send + Sync>;

/// Everything that can go wrong reaching a daemon, pairing a host or running the relay.
#[derive(Debug, Error)]
#[non_exhaustive]
pub enum Error {
    /// The host id cannot name a registered remote host.
    #[error("host id {id} cannot name a remote host: {why}")]
    BadHostId {
        /// The id that was refused.
        id: HostId,
        /// What is wrong with it.
        why: &'static str,
    },
    /// An identifier is malformed.
    #[error(transparent)]
    Id(#[from] protocol::Error),
    /// The text is not `host:port`.
    #[error("{text:?} is not a host:port address: {why}")]
    MalformedEndpoint {
        /// What was typed.
        text: String,
        /// What is wrong with it.
        why: &'static str,
    },
    /// The host name did not resolve to any address.
    #[error("cannot resolve {host}: {reason}")]
    Unresolvable {
        /// The name that was looked up.
        host: String,
        /// The resolver's answer.
        reason: String,
    },
    /// No resolved address accepted a connection.
    #[error("cannot reach {endpoint}: {source}")]
    Unreachable {
        /// The address that was dialled.
        endpoint: Endpoint,
        /// The last connect error.
        #[source]
        source: io::Error,
    },
    /// The origin may not reach the path.
    #[error("this origin may not reach {0:?}")]
    Forbidden(String),
    /// The hosts file could not be read or written.
    #[error("hosts file {path}: {source}")]
    HostsIo {
        /// The file involved.
        path: PathBuf,
        /// The OS error.
        #[source]
        source: io::Error,
    },
    /// The hosts file is not valid JSON of the expected shape.
    #[error("hosts file {path} is corrupt: {source}")]
    HostsCorrupt {
        /// The file involved.
        path: PathBuf,
        /// The parse error.
        #[source]
        source: serde_json::Error,
    },
    /// An entry of the hosts file cannot be used.
    #[error("hosts file {path} holds a bad entry: {why}")]
    HostsInvalid {
        /// The file involved.
        path: PathBuf,
        /// Which entry and what is wrong with it.
        why: String,
    },
    /// The hosts file was written by a newer Crowbar.
    #[error("hosts file {path} has version {found}, this Crowbar reads version {supported}")]
    HostsVersion {
        /// The file involved.
        path: PathBuf,
        /// The version it declares.
        found: u32,
        /// The version this build reads.
        supported: u32,
    },
    /// The credential store failed.
    #[error("{0}")]
    Store(String),
    /// A host did not answer: refused, timed out or dropped the handshake.
    #[error("{0}")]
    HostUnreachable(String),
    /// A host answered, but not with the identity pinned at pairing.
    #[error("{0}")]
    IdentityChanged(String),
    /// The HTTP exchange failed.
    #[error("http exchange failed: {0}")]
    Exchange(#[source] Cause),
    /// The request could not be built.
    #[error("bad request: {0}")]
    BadRequest(#[source] Cause),
    /// The answer passed the size cap.
    #[error("response too large")]
    TooLarge,
    /// The answer's body failed mid-stream.
    #[error("reading the response: {0}")]
    Body(String),
    /// The host is not registered.
    #[error("unknown remote host {0}")]
    UnknownHost(HostId),
    /// Pairing stored nothing usable for the host.
    #[error("no stored credential for this host \u{2014} pair it again")]
    NoCredential,
    /// The stored token cannot be sent as a header.
    #[error("the stored credential is not a valid header value")]
    BadToken,
    /// The blocking credential or registry task died.
    #[error("blocking task failed: {0}")]
    BlockingTask(String),
    /// The probe did not finish in time.
    #[error("{0} did not answer within the probe deadline")]
    ProbeTimeout(Endpoint),
    /// A TLS handshake failed.
    #[error("TLS handshake with {endpoint} failed: {reason}")]
    Handshake {
        /// The host dialled.
        endpoint: Endpoint,
        /// What went wrong.
        reason: String,
    },
    /// The handshake completed without a certificate.
    #[error("{0} presented no certificate")]
    NoCertificate(Endpoint),
    /// Pairing did not finish in time.
    #[error("{0} did not finish pairing in time")]
    PairTimeout(Endpoint),
    /// The exchange with a host being paired failed.
    #[error("pairing with {endpoint} failed: {source}")]
    PairExchange {
        /// The host paired with.
        endpoint: Endpoint,
        /// The failure.
        #[source]
        source: Box<Error>,
    },
    /// The host declined the request.
    #[error("the host refused ({status}): {reason}")]
    Refused {
        /// The HTTP status.
        status: u16,
        /// The host's stated reason.
        reason: String,
    },
    /// The host's pairing answer lacked a field.
    #[error("the host's pairing answer carried no {0}")]
    MissingField(&'static str),
    /// Pairing redeemed a token but could not record it; nothing is left behind.
    #[error("pairing with {endpoint} could not be recorded ({stage}: {cause}); {cleanup}")]
    NotRecorded {
        /// The host paired with.
        endpoint: Endpoint,
        /// What could not be written: the host list or the keychain.
        stage: &'static str,
        /// The store's failure.
        cause: String,
        /// How undoing the pairing went.
        cleanup: String,
    },
    /// The host was removed, but its stored credential could not be deleted.
    #[error("removed host {host}, but its stored credential remains: {source}")]
    CredentialRemains {
        /// The host that was removed.
        host: HostId,
        /// Why the credential could not be deleted.
        #[source]
        source: Box<Error>,
    },
    /// The local daemon has not published its socket yet.
    #[error("the local daemon has not published a socket")]
    NoSocket,
    /// The local daemon's socket could not be connected.
    #[error("cannot connect to the local daemon socket: {0}")]
    LocalSocket(#[source] io::Error),
    /// The upstream did not complete a WebSocket upgrade in time.
    #[error("the daemon did not answer the upgrade in time")]
    UpgradeTimeout,
    /// A WebSocket upgrade or frame failed.
    #[error("websocket: {0}")]
    WebSocket(#[source] Cause),
    /// The relay could not bind its loopback port.
    #[error("relay cannot bind 127.0.0.1: {0}")]
    Bind(#[source] io::Error),
    /// A request from a pane is malformed.
    #[error("invalid request: {0}")]
    InvalidRequest(&'static str),
    /// The system random source failed.
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
#[path = "error_tests.rs"]
mod tests;
