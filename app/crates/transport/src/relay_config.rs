use std::time::Duration;

const DEFAULT_MAX_CONNECTIONS: usize = 256;
const DEFAULT_MAX_PER_PANE: usize = 32;
const DEFAULT_MAX_HANDSHAKES: usize = 64;
const DEFAULT_HANDSHAKE_SECS: u64 = 3;
const DEFAULT_IO_TIMEOUT_SECS: u64 = 10;
const DEFAULT_TOKEN_TTL_MINUTES: u64 = 5;
const DEFAULT_MAX_MESSAGE_BYTES: usize = 16 * 1024 * 1024;

/// Limits and timeouts of the loopback relay.
///
/// Open streams are counted against `max_connections` and `max_per_pane` only once the token has
/// been checked; connections still in their handshake are counted separately against
/// `max_handshakes` and give the slot back the moment the handshake ends, so a client that
/// never authenticates cannot take a stream's place.
#[derive(Debug, Clone)]
pub struct RelayConfig {
    /// Open streams across all panes.
    pub max_connections: usize,
    /// Open streams of one pane.
    pub max_per_pane: usize,
    /// Connections that may be in their handshake at once; further ones are answered `503`.
    pub max_handshakes: usize,
    /// Time a client has to finish the WebSocket handshake.
    pub handshake_timeout: Duration,
    /// Time the daemon has to accept the upstream WebSocket upgrade.
    pub open_timeout: Duration,
    /// Time a peer has to accept one outgoing message before it counts as stalled and its
    /// stream is closed.
    pub write_timeout: Duration,
    /// Time both sides together have to complete a close before the stream is dropped.
    pub close_grace: Duration,
    /// How long an issued token stays valid.
    pub token_ttl: Duration,
    /// Largest message in either direction; a terminal snapshot frame is the biggest honest one.
    pub max_message_bytes: usize,
    /// Close a stream whose daemon says nothing for this long, unless the pane offered its own
    /// `crowbar.idle.<seconds>` subprotocol; for streams the daemon pings. `None` never closes.
    pub upstream_idle: Option<Duration>,
    /// Close a stream whose pane says nothing for this long. `None` never closes.
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
    fn the_defaults_leave_room_for_several_panes_and_bound_every_wait() {
        let config = RelayConfig::default();
        assert!(config.max_per_pane < config.max_connections);
        assert!(config.handshake_timeout > Duration::ZERO);
        assert!(config.token_ttl > config.handshake_timeout);
        assert!(
            config.token_ttl <= Duration::from_mins(15),
            "a token only has to outlive the connect that uses it"
        );
        assert!(
            config.max_message_bytes >= 16 * 1024 * 1024,
            "a terminal snapshot must fit"
        );
    }

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
