use super::*;

#[test]
fn requires_a_namespace() {
    for ok in ["core.chat", "ext.git.review", "core.a.b", "ext.my-ext.x_y"] {
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
fn an_extension_segment_that_is_no_extension_id_is_refused() {
    for bad in ["ext.my_ext.bar", "ext.1foo.bar", "ext.-x.bar"] {
        assert!(PaneTypeId::new(bad).is_err(), "{bad} should be rejected");
    }
}

#[test]
fn knows_its_owner() {
    let core = PaneTypeId::new("core.terminal").unwrap();
    assert_eq!(core.kind(), PaneKind::Core, "core kind");
    assert_eq!(core.extension(), None, "core has no extension");
    assert_eq!(core.origin(), &PaneOrigin::Core, "core origin");
    let ext = PaneTypeId::new("ext.git.review").unwrap();
    assert_eq!(ext.kind(), PaneKind::Extension, "extension kind");
    let git = ExtensionId::new("git").unwrap();
    assert_eq!(ext.extension(), Some(git.clone()), "owner");
    assert_eq!(
        ext.origin(),
        &PaneOrigin::Extension(git),
        "extension origin"
    );
}

#[test]
fn text_and_serde_round_trip_and_validate() {
    let id: PaneTypeId = "ext.git.review".parse().unwrap();
    assert_eq!(id.to_string(), "ext.git.review", "display");
    assert_eq!(id.as_str(), "ext.git.review", "as_str");
    assert_eq!(id.as_ref(), "ext.git.review", "as_ref");
    let json = serde_json::to_string(&id).unwrap();
    assert_eq!(json, "\"ext.git.review\"", "serialize");
    assert_eq!(
        serde_json::from_str::<PaneTypeId>(&json).unwrap(),
        id,
        "round trip"
    );
    assert!(
        serde_json::from_str::<PaneTypeId>("\"ext.my_ext.x\"").is_err(),
        "validates"
    );
    assert_eq!(String::from(id), "ext.git.review", "into string");
}
