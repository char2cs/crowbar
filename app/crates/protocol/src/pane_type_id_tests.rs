use super::*;

#[test]
fn requires_a_namespace() {
    for ok in ["core.chat", "ext.git.review", "core.a.b"] {
        assert!(PaneTypeId::new(ok).is_ok(), "{ok} should be valid");
    }
    for bad in [
        "chat",
        "ext.git",
        "other.thing",
        "core..x",
        "core.",
        ".core.x",
        "Core.chat",
    ] {
        assert!(PaneTypeId::new(bad).is_err(), "{bad} should be rejected");
    }
}

#[test]
fn knows_its_owner() {
    let core = PaneTypeId::new("core.terminal").unwrap();
    assert_eq!(core.kind(), PaneKind::Core, "core kind");
    assert_eq!(core.extension(), None, "core has no extension");
    let ext = PaneTypeId::new("ext.git.review").unwrap();
    assert_eq!(ext.kind(), PaneKind::Extension, "extension kind");
    assert_eq!(
        ext.extension(),
        Some(ExtensionId::new("git").unwrap()),
        "owner"
    );
}
