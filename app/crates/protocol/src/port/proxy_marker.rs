//! The `x-crowbar-proxy` markers: an answer the shell's transport gave in the daemon's place.
//!
//! Spec: develop `desktop/src-tauri/src/api_proxy.rs` and `web/src/lib/api/errors.ts`.

/// Marks a response that is the proxy's, not the daemon's; the value says why.
pub const PROXY_HEADER: &str = "x-crowbar-proxy";
/// The local daemon socket: nothing was listening.
pub const DAEMON_UNAVAILABLE: &str = "daemon-unavailable";
/// A remote host did not answer: refused, timed out, or dropped the handshake.
pub const HOST_UNREACHABLE: &str = "host-unreachable";
/// A remote host answered, but not with the identity pinned at pairing.
pub const HOST_IDENTITY_CHANGED: &str = "host-identity-changed";
/// No credential is stored for the host: pairing has to be done again.
pub const NO_CREDENTIAL: &str = "no-credential";
/// The host id is not registered.
pub const UNKNOWN_HOST: &str = "unknown-host";
