use super::*;

#[test]
fn keeps_value_and_label_apart() {
    let choice = Choice::new("system", "Sync with System");
    assert_eq!(
        (choice.value(), choice.label()),
        ("system", "Sync with System")
    );
}
