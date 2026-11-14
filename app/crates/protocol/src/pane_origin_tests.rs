use super::*;

#[test]
fn an_extension_origin_names_its_extension_and_core_names_none() {
    let id = ExtensionId::new("git").unwrap();
    assert_eq!(PaneOrigin::Core.extension(), None, "core");
    assert_eq!(
        PaneOrigin::Extension(id.clone()).extension(),
        Some(&id),
        "extension"
    );
}
