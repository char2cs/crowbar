//! Daemon transport for Crowbar clients, with no window system and no Tauri.
//!
//! Panes reach a daemon only through this crate. HTTP goes through [`Transport::handle`], a pure
//! async function from a `crowbar://<hostId>/v0/...` request to its response, over the local
//! daemon's unix socket or a paired remote host's pinned TLS. WebSocket streams go through the
//! loopback [`Relay`], which a webview dials with its native `WebSocket` so binary frames stay
//! binary. [`ShellTransport`] implements `protocol::DaemonTransport` over both. Replacing the
//! relay with daemon-issued tokens changes only the [`StreamAccess`] implementor.

mod access_scope;
mod admission;
mod async_stream;
mod blocking;
mod bridge;
mod bridge_limits;
mod cached_token_store;
mod connection_book;
mod connection_guard;
mod credential;
mod credential_io;
mod daemon_wire;
mod dial;
mod ending;
mod endpoint;
mod error;
mod exact_origins;
mod fingerprint;
mod first_connected;
mod gate;
mod host_entry;
mod host_limits;
mod host_registry;
mod host_target;
mod hosts_document;
mod http_conn;
mod keychain;
mod live;
mod local;
mod local_get;
mod memory_token_store;
mod origin_policy;
mod outcome;
mod pair_request;
mod paired_host;
mod pairing;
mod pin;
mod pinned_verifier;
mod probe;
mod proxy_answer;
mod read_error;
mod redeemed;
mod registry;
mod relay;
mod relay_config;
mod relay_protocol;
mod relay_state;
mod remote;
mod remote_host;
mod reply;
mod response;
mod session;
mod shell_transport;
mod socket_locator;
mod stream_access;
mod tls_config;
mod token_book;
mod token_store;
mod transport;
mod unpinned_probe_verifier;
mod unrecorded;
mod upstream;
mod upstream_request;

#[cfg(feature = "os-keychain")]
mod os_keychain;

pub use access_scope::EXTENSION_HEADER;
pub use cached_token_store::CachedTokenStore;
pub use endpoint::Endpoint;
pub use error::Error;
pub use exact_origins::ExactOrigins;
pub use fingerprint::fingerprint_of;
pub use host_registry::HostRegistry;
pub use keychain::Keychain;
pub use local_get::local_get;
pub use memory_token_store::MemoryTokenStore;
pub use origin_policy::OriginPolicy;
pub use pair_request::PairRequest;
pub use pairing::pair_remote_host;
pub use probe::probe_fingerprint;
pub use read_error::ReadError;
pub use relay::Relay;
pub use relay_config::RelayConfig;
pub use relay_protocol::{IDLE_SUBPROTOCOL_PREFIX, RELAY_SUBPROTOCOL, TOKEN_SUBPROTOCOL_PREFIX};
pub use response::{
    DAEMON_UNAVAILABLE, HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, NO_CREDENTIAL, PROXY_HEADER,
    UNKNOWN_HOST, marker_of,
};
pub use shell_transport::ShellTransport;
pub use socket_locator::SocketLocator;
pub use stream_access::StreamAccess;
pub use token_store::TokenStore;
pub use transport::Transport;

#[cfg(feature = "os-keychain")]
pub use os_keychain::OsKeychain;
