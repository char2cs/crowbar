use super::*;

#[test]
fn builders_override_the_defaults() {
    let list = ListBlock::new("agents", "No agents available.")
        .titled("Descriptor checks")
        .intro("Drag to reorder.")
        .states("Loading agents...", "Could not load agents.")
        .columns(&["Installed", "Tools", "Enabled"]);
    assert_eq!(list.source, "agents");
    assert_eq!(list.title.as_deref(), Some("Descriptor checks"));
    assert_eq!(list.intro.as_deref(), Some("Drag to reorder."));
    assert_eq!(
        (list.loading.as_str(), list.failed.as_str()),
        ("Loading agents...", "Could not load agents.")
    );
    assert_eq!(list.columns, ["Installed", "Tools", "Enabled"]);
}
