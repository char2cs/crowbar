use std::sync::Arc;

use protocol::{HostId, PaneId, PaneOrigin};

use super::*;

struct Fixed;

impl StreamAccess for Fixed {
    fn issue(&self, scope: &RelayScope) -> Result<RelayEndpoint, Error> {
        Ok(RelayEndpoint::new(
            "ws://127.0.0.1:1",
            scope.pane().to_string(),
        ))
    }

    fn revoke_pane(&self, _pane: &PaneId) {}
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
