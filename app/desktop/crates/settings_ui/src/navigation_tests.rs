use super::*;

fn ids() -> Vec<PageId> {
    ["appearance", "editor", "git"].map(PageId::from).to_vec()
}

fn after(start: &str, movement: RailMove) -> String {
    let ids = ids();
    let visible: Vec<&PageId> = ids.iter().collect();
    let mut navigation = Navigation::new(PageId::from(start));
    navigation.step(movement, &visible);
    navigation.active().to_string()
}

#[test]
fn arrows_wrap_and_home_end_jump() {
    const CASES: &[(&str, RailMove, &str)] = &[
        ("appearance", RailMove::Next, "editor"),
        ("git", RailMove::Next, "appearance"),
        ("appearance", RailMove::Previous, "git"),
        ("editor", RailMove::Previous, "appearance"),
        ("editor", RailMove::First, "appearance"),
        ("editor", RailMove::Last, "git"),
        ("hidden", RailMove::Next, "appearance"),
        ("hidden", RailMove::Previous, "git"),
    ];
    for &(start, movement, expected) in CASES {
        assert_eq!(after(start, movement), expected, "{start} {movement:?}");
    }
}

#[test]
fn select_ignores_pages_that_are_not_visible_and_reconcile_falls_back() {
    let ids = ids();
    let visible: Vec<&PageId> = ids.iter().skip(1).collect();
    let mut navigation = Navigation::new(PageId::from("appearance"));
    navigation.select(&PageId::from("appearance"), &visible);
    assert_eq!(
        navigation.active().as_str(),
        "appearance",
        "hidden pages cannot be selected"
    );
    navigation.reconcile(&visible);
    assert_eq!(
        navigation.active().as_str(),
        "editor",
        "a hidden active page moves to the first visible"
    );
    navigation.select(&PageId::from("git"), &visible);
    assert_eq!(navigation.active().as_str(), "git");
    navigation.reconcile(&[]);
    assert_eq!(
        navigation.active().as_str(),
        "git",
        "nothing visible keeps the selection"
    );
}
