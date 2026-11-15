//! What the user typed and confirmed in the pairing dialog.

/// The host to register and the code that proves the user may.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct PairingRequest {
    /// The id to register the host under.
    pub host_id: String,
    /// The host's canonical `host:port`.
    pub endpoint: String,
    /// The fingerprint the user confirmed.
    pub fingerprint: String,
    /// The one-time pairing code.
    pub code: String,
    /// The name the host shows for this client.
    pub name: String,
}
