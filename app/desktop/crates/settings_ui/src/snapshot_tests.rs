use super::*;

#[test]
fn builders_collect_capabilities_and_choices() {
    let snapshot = Snapshot::new(Values::default().with("a", true))
        .with_capability("browser.clearData")
        .with_choices("themes", vec![Choice::new("crowbar", "Crowbar")]);
    assert!(snapshot.capabilities.contains("browser.clearData"));
    assert_eq!(snapshot.choices.get("themes").map(Vec::len), Some(1));
    assert!(snapshot.values.get(&"a".into()).is_some());
}
