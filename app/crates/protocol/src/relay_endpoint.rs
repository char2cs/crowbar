use serde::{Deserialize, Serialize};

/// A loopback WebSocket endpoint a pane can dial with its native `WebSocket`.
#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct RelayEndpoint {
    /// Base URL, for example `ws://127.0.0.1:41233`.
    pub url: String,
    /// Per-pane session token the relay checks together with the `Origin`.
    pub token: String,
}
