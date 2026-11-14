use super::*;

#[test]
fn getters_return_what_was_built() {
    let scope = RelayScope::new(
        HostId::new("host-1").unwrap(),
        PaneId::new("p1").unwrap(),
        PaneOrigin::Core,
    );
    assert_eq!(scope.host().as_str(), "host-1", "host");
    assert_eq!(scope.pane().as_str(), "p1", "pane");
    assert_eq!(scope.origin(), &PaneOrigin::Core, "origin");
}
