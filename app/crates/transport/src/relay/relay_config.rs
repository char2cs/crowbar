use std::time::Duration;

const DEFAULT_MAX_CONNECTIONS: usize = 256;
const DEFAULT_MAX_PER_PANE: usize = 32;
const DEFAULT_MAX_HANDSHAKES: usize = 64;
const DEFAULT_HANDSHAKE_SECS: u64 = 3;
const DEFAULT_IO_TIMEOUT_SECS: u64 = 10;
const DEFAULT_TOKEN_TTL_MINUTES: u64 = 5;
const DEFAULT_MAX_MESSAGE_BYTES: usize = 16 * 1024 * 1024;

#[derive(Debug, Clone)]
pub struct RelayConfig {
    pub max_connections: usize,
    pub max_per_pane: usize,
    pub max_handshakes: usize,
    pub handshake_timeout: Duration,
    pub open_timeout: Duration,
    pub write_timeout: Duration,
    pub close_grace: Duration,
    pub token_ttl: Duration,
    pub max_message_bytes: usize,
    pub upstream_idle: Option<Duration>,
    pub client_idle: Option<Duration>,
}

impl Default for RelayConfig {
    fn default() -> Self {
        Self {
            max_connections: DEFAULT_MAX_CONNECTIONS,
            max_per_pane: DEFAULT_MAX_PER_PANE,
            max_handshakes: DEFAULT_MAX_HANDSHAKES,
            handshake_timeout: Duration::from_secs(DEFAULT_HANDSHAKE_SECS),
            open_timeout: Duration::from_secs(DEFAULT_IO_TIMEOUT_SECS),
            write_timeout: Duration::from_secs(DEFAULT_IO_TIMEOUT_SECS),
            close_grace: Duration::from_secs(2),
            token_ttl: Duration::from_mins(DEFAULT_TOKEN_TTL_MINUTES),
            max_message_bytes: DEFAULT_MAX_MESSAGE_BYTES,
            upstream_idle: None,
            client_idle: None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn handshakes_are_short_and_bounded_apart_from_streams() {
        let config = RelayConfig::default();
        assert!(
            config.handshake_timeout <= Duration::from_secs(5),
            "pre-auth clients are cheap to drop"
        );
        assert!(
            config.max_handshakes < config.max_connections,
            "handshakes never outnumber streams"
        );
        assert!(config.close_grace < config.write_timeout);
    }
}
