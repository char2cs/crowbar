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
mod bridge;
mod cached_token_store;
mod connection_book;
mod connection_guard;
mod credential;
mod daemon_wire;
mod dial;
mod ending;
mod endpoint;
mod error;
mod exact_origins;
mod fingerprint;
mod gate;
mod host_entry;
mod host_path;
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
mod upstream;

#[cfg(feature = "os-keychain")]
mod os_keychain;

pub use access_scope::{AccessScope, EXTENSION_HEADER};
pub use async_stream::{AsyncStream, BoxedStream};
pub use cached_token_store::CachedTokenStore;
pub use credential::Credential;
pub use dial::{DIAL_TIMEOUT, KEEPALIVE_IDLE, KEEPALIVE_INTERVAL, dial};
pub use endpoint::Endpoint;
pub use error::Error;
pub use exact_origins::ExactOrigins;
pub use fingerprint::{CertificateDigest, certificate_digest, fingerprint_of, hex};
pub use host_path::{HostPath, qualify, split_host_path};
pub use host_registry::HostRegistry;
pub use host_target::HostTarget;
pub use http_conn::{Channel, collect_capped, exchange, open_channel, send};
pub use keychain::Keychain;
pub use local::{CONNECT_ATTEMPTS, connect, connect_once, connect_retry_delay, forward_local};
pub use local_get::local_get;
pub use memory_token_store::MemoryTokenStore;
pub use origin_policy::OriginPolicy;
pub use pair_request::PairRequest;
pub use paired_host::PairedHost;
pub use pairing::{PAIR_TIMEOUT, pair_remote_host};
pub use pin::Pin;
pub use pinned_verifier::pinned_client_config;
pub use probe::{PROBE_TIMEOUT, probe_fingerprint};
pub use read_error::ReadError;
pub use relay::Relay;
pub use relay_config::RelayConfig;
pub use relay_protocol::{RELAY_SUBPROTOCOL, TOKEN_SUBPROTOCOL_PREFIX};
pub use remote::{REMOTE_BODY_LIMIT, bearer, forward_remote, paired_host, stored_credential};
pub use remote_host::RemoteHost;
pub use response::{
    DAEMON_UNAVAILABLE, HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, PROXY_HEADER, error_response,
    marked_response, marker_of, readable_by,
};
pub use shell_transport::ShellTransport;
pub use socket_locator::SocketLocator;
pub use stream_access::StreamAccess;
pub use token_store::TokenStore;
pub use transport::{PROXY_TIMEOUT, Transport};
pub use upstream::UpstreamSocket;

#[cfg(feature = "os-keychain")]
pub use os_keychain::OsKeychain;
