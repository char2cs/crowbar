use std::io;
use std::path::PathBuf;

use protocol::HostId;
use thiserror::Error;

use crate::endpoint::Endpoint;

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
    /// A host-qualified path is malformed.
    #[error("bad host path {path:?}: {why}")]
    BadHostPath {
        /// The path that was refused.
        path: String,
        /// What is wrong with it.
        why: &'static str,
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
    /// The hosts file is not valid.
    #[error("hosts file {path} is corrupt: {source}")]
    HostsCorrupt {
        /// The file involved.
        path: PathBuf,
        /// The parse error.
        #[source]
        source: serde_json::Error,
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
    #[error("{0}")]
    Http(#[from] hyper::Error),
    /// The request could not be built.
    #[error("bad request: {0}")]
    BadRequest(#[from] http::Error),
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
    /// The blocking credential task died.
    #[error("token lookup failed: {0}")]
    CredentialTask(String),
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
        status: http::StatusCode,
        /// The host's stated reason.
        reason: String,
    },
    /// The host's pairing answer lacked a field.
    #[error("the host's pairing answer carried no {0}")]
    MissingField(&'static str),
    /// The keychain could not keep the token.
    #[error("the keychain could not keep the token for {endpoint} ({keychain}); {revoke}")]
    Unkept {
        /// The host paired with.
        endpoint: Endpoint,
        /// The keychain's failure.
        keychain: String,
        /// How revoking the token on the host went.
        revoke: String,
    },
    /// The local daemon's socket could not be connected.
    #[error("connect daemon socket: {0}")]
    LocalSocket(String),
    /// The remote did not complete a WebSocket upgrade in time.
    #[error("remote host did not answer the upgrade in time")]
    UpgradeTimeout,
    /// A WebSocket upgrade or frame failed.
    #[error("websocket: {0}")]
    WebSocket(#[from] Box<tokio_tungstenite::tungstenite::Error>),
    /// The relay could not bind its loopback port.
    #[error("relay cannot bind 127.0.0.1: {0}")]
    Bind(#[source] io::Error),
    /// A relay grant was refused.
    #[error("relay grant refused: {0}")]
    BadGrant(&'static str),
    /// The system random source failed.
    #[error("random source: {0}")]
    Random(String),
}

impl Error {
    pub(crate) fn marker(&self) -> Option<&'static str> {
        match self {
            Self::HostUnreachable(_) | Self::UpgradeTimeout => {
                Some(crate::response::HOST_UNREACHABLE)
            },
            Self::IdentityChanged(_) => Some(crate::response::HOST_IDENTITY_CHANGED),
            _ => None,
        }
    }
}

#[cfg(test)]
#[path = "error_tests.rs"]
mod tests;
