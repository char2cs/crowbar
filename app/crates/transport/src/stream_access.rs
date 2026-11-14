use protocol::{PaneId, RelayEndpoint, RelayScope};

use crate::error::Error;

/// How panes get WebSocket access to a daemon. The loopback relay implements it today; a
/// daemon-issued token scheme can replace it later without touching callers.
pub trait StreamAccess: Send + Sync {
    /// Issues the endpoint and token the pane dials with its native `WebSocket`, offering the
    /// subprotocols [`RELAY_SUBPROTOCOL`](crate::RELAY_SUBPROTOCOL) and
    /// `crowbar.token.<token>`. Issuing again for the same pane and host replaces the old token.
    ///
    /// # Errors
    /// [`Error::BadGrant`] for a scope that contradicts itself, [`Error::Random`] when no
    /// token can be generated.
    fn issue(&self, scope: &RelayScope) -> Result<RelayEndpoint, Error>;

    /// Ends every token and open stream of the pane, for when it is destroyed.
    fn revoke_pane(&self, pane: &PaneId);
}

#[cfg(test)]
#[path = "stream_access_tests.rs"]
mod tests;
