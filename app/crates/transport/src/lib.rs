mod blocking;
mod error;
mod hosts;
mod journal_sink;
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
