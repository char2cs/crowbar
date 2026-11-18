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
    /// [`Error::Random`] when no token can be generated.
    fn issue(
        &self,
        scope: &RelayScope,
    ) -> Result<RelayEndpoint, Error>;

    /// Ends every token and open stream of the pane, for when it is destroyed.
    fn revoke_pane(
        &self,
        pane: &PaneId,
    );
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use protocol::{HostId, PaneId, PaneOrigin};

    use super::*;

    struct Fixed;

    impl StreamAccess for Fixed {
        fn issue(
            &self,
            scope: &RelayScope,
        ) -> Result<RelayEndpoint, Error> {
            Ok(RelayEndpoint::new(
                "ws://127.0.0.1:1",
                scope.pane().to_string(),
            ))
        }

        fn revoke_pane(
            &self,
            _pane: &PaneId,
        ) {
        }
    }

    #[test]
    fn an_implementor_can_replace_the_relay_behind_a_shared_trait_object() {
        let access: Arc<dyn StreamAccess> = Arc::new(Fixed);
        let scope = RelayScope::new(
            HostId::new("local").unwrap(),
            PaneId::new("p1").unwrap(),
            PaneOrigin::Core,
        );
        assert_eq!(access.issue(&scope).unwrap().token(), "p1");
        access.revoke_pane(scope.pane());
    }
}
