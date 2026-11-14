/// The WebSocket subprotocol a pane offers and the relay selects.
pub const RELAY_SUBPROTOCOL: &str = "crowbar.relay.v1";

/// Prefix of the second offered subprotocol, which carries the pane's token:
/// `new WebSocket(url, ["crowbar.relay.v1", "crowbar.token.<token>"])`. A browser `WebSocket`
/// cannot set headers, and a subprotocol keeps the token out of URLs, logs and referrers.
pub const TOKEN_SUBPROTOCOL_PREFIX: &str = "crowbar.token.";
