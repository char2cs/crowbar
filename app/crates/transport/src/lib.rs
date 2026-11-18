//! Daemon transport for Crowbar clients, with no window system and no Tauri.
//!
//! Panes reach a daemon only through this crate. HTTP goes through [`Transport::handle`], a pure
//! async function from a `crowbar://<hostId>/v0/...` request to its response, over the local
//! daemon's unix socket or a paired remote host's pinned TLS. WebSocket streams go through the
//! loopback [`Relay`], which a webview dials with its native `WebSocket` so binary frames stay
//! binary. [`ShellTransport`] implements `protocol::DaemonTransport` over both. Replacing the
//! relay with daemon-issued tokens changes only the [`StreamAccess`] implementor.

mod blocking;
mod error;
mod hosts;
mod pairing;
mod relay;
mod response;
mod transport;

pub use error::Error;
pub use hosts::{Endpoint, HostRegistry};
pub use pairing::{PairRequest, pair_remote_host, probe_fingerprint};
pub use relay::{
    Acceptor, ExactOrigins, IDLE_SUBPROTOCOL_PREFIX, OriginPolicy, RELAY_SUBPROTOCOL, Relay,
    RelayConfig, TOKEN_SUBPROTOCOL_PREFIX,
};
pub use response::{
    DAEMON_UNAVAILABLE, HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, NO_CREDENTIAL, PROXY_HEADER,
    UNKNOWN_HOST, marker_of,
};
pub use transport::{
    CachedTokenStore, EXTENSION_HEADER, Keychain, MemoryTokenStore, ReadError, ShellTransport,
    SocketLocator, StreamAccess, TextStream, TokenStore, Transport, fingerprint_of, local_get,
};

#[cfg(feature = "os-keychain")]
pub use transport::OsKeychain;
