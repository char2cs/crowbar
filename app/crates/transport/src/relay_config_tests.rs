use super::*;

#[test]
fn the_defaults_leave_room_for_several_panes_and_bound_every_wait() {
    let config = RelayConfig::default();
    assert!(config.max_per_pane < config.max_connections);
    assert!(config.handshake_timeout > Duration::ZERO);
    assert!(config.token_ttl > config.handshake_timeout);
    assert!(
        config.max_message_bytes >= 16 * 1024 * 1024,
        "a terminal snapshot must fit"
    );
}
