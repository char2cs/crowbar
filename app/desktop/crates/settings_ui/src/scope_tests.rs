use super::*;

#[test]
fn stored_values_win_over_defaults() {
    let values = Values::default().with("a", true);
    let defaults = Values::default().with("a", false).with("b", 3.0);
    let capabilities = BTreeSet::new();
    let scope = Scope {
        values: &values,
        defaults: &defaults,
        capabilities: &capabilities,
    };
    assert_eq!(scope.lookup(&Key::from("a")), Some(&Value::Bool(true)));
    assert_eq!(scope.lookup(&Key::from("b")), Some(&Value::Number(3.0)));
    assert_eq!(scope.lookup(&Key::from("c")), None);
}
