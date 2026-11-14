use super::*;

#[test]
fn an_empty_default_is_unbound() {
    assert_eq!(Shortcut::new("tabs.new", "New tab", "").default, None);
    assert_eq!(
        Shortcut::new("tabs.new", "New tab", "Mod+T")
            .default
            .as_ref()
            .map(Chord::as_str),
        Some("mod+t")
    );
    assert_eq!(
        Shortcut::new("tabs.new", "New tab", "").command(),
        "tabs.new"
    );
}
