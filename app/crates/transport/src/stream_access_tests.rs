use std::sync::Arc;

use protocol::{HostId, PaneId, PaneKind};

use super::*;

struct Fixed;

impl StreamAccess for Fixed {
    fn issue(&self, scope: &RelayScope) -> Result<RelayEndpoint, Error> {
        Ok(RelayEndpoint {
            url: "ws://127.0.0.1:1".into(),
            token: scope.pane.to_string(),
        })
    }

    fn revoke_pane(&self, _pane: &PaneId) {}
}

#[test]
fn an_implementor_can_replace_the_relay_behind_a_shared_trait_object() {
    let access: Arc<dyn StreamAccess> = Arc::new(Fixed);
    let scope = RelayScope {
        host: HostId::new("local").unwrap(),
        pane: PaneId::new("p1").unwrap(),
        kind: PaneKind::Core,
        extension: None,
    };
    assert_eq!(access.issue(&scope).unwrap().token, "p1");
    access.revoke_pane(&scope.pane);
}
