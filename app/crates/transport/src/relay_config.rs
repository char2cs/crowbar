use std::time::Duration;

/// Limits and timeouts of the loopback relay.
#[derive(Debug, Clone)]
pub struct RelayConfig {
    /// Open client connections across all panes.
    pub max_connections: usize,
    /// Open client connections of one pane.
    pub max_per_pane: usize,
    /// Time a client has to finish the WebSocket handshake.
    pub handshake_timeout: Duration,
    /// How long an issued token stays valid.
    pub token_ttl: Duration,
    /// Largest message in either direction; a terminal snapshot frame is the biggest honest one.
    pub max_message_bytes: usize,
    /// Close a stream whose daemon says nothing for this long; for streams the daemon pings.
    pub upstream_idle: Option<Duration>,
    /// Close a stream whose pane says nothing for this long.
    pub client_idle: Option<Duration>,
}

impl Default for RelayConfig {
    fn default() -> Self {
        Self {
            max_connections: 256,
            max_per_pane: 32,
            handshake_timeout: Duration::from_secs(10),
            token_ttl: Duration::from_hours(12),
            max_message_bytes: 64 * 1024 * 1024,
            upstream_idle: None,
            client_idle: None,
        }
    }
}

#[cfg(test)]
#[path = "relay_config_tests.rs"]
mod tests;
