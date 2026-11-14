use super::*;

#[test]
fn core_and_extension_types_map_to_their_origin() {
    let core = Origin::of(&PaneTypeId::new("core.chat").unwrap());
    assert_eq!((core.kind, core.extension), (PaneKind::Core, None), "core");
    let ext = Origin::of(&PaneTypeId::new("ext.git.review").unwrap());
    assert_eq!(ext.kind, PaneKind::Extension, "kind");
    assert_eq!(ext.extension, ExtensionId::new("git").ok(), "extension");
}
