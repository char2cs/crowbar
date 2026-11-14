use std::sync::Arc;

use protocol::{HostId, PaneId, PaneOrigin};

use super::*;

struct OnlyThis(&'static str);

impl OriginPolicy for OnlyThis {
    fn allows(&self, _scope: &RelayScope, origin: &str) -> bool {
        origin == self.0
    }
}

#[test]
fn a_policy_is_usable_as_a_shared_trait_object() {
    let policy: Arc<dyn OriginPolicy> = Arc::new(OnlyThis("crowbar://app"));
    let scope = RelayScope::new(
        HostId::new("local").unwrap(),
        PaneId::new("p1").unwrap(),
        PaneOrigin::Core,
    );
    assert!(policy.allows(&scope, "crowbar://app"));
    assert!(!policy.allows(&scope, "crowbar://other"));
}
