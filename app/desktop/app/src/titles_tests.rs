use super::*;

#[test]
fn clones_share_titles_and_retain_prunes() {
    let titles = Titles::default();
    let copy = titles.clone();
    let (a, b) = (PaneId::new("a").unwrap(), PaneId::new("b").unwrap());
    titles.set(a.clone(), PaneTitle::new("A").unwrap());
    copy.set(b.clone(), PaneTitle::new("B").unwrap());
    assert_eq!(
        copy.get(&a).map(|t| t.to_string()),
        Some("A".into()),
        "shared"
    );
    titles.retain(|pane| *pane == b);
    assert!(copy.get(&a).is_none(), "pruned");
    assert!(titles.get(&b).is_some(), "kept");
}
