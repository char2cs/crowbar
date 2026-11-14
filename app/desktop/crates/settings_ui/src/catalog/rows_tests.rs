use super::*;

#[test]
fn rows_carry_label_description_and_default() {
    let built = row(
        ("wordWrap", "Word Wrap"),
        "Wrap lines",
        Control::switch(),
        false,
    );
    assert_eq!(
        (built.key().as_str(), built.label()),
        ("wordWrap", "Word Wrap")
    );
    assert_eq!(built.description.as_deref(), Some("Wrap lines"));
    assert_eq!(built.default, Some(Value::Bool(false)));
}

#[test]
fn snapped_numbers_snap_and_plain_numbers_do_not() {
    let snapping = snapped((10.0, 24.0, 0.5), ControlWidth::Compact);
    let plain = number((10.0, 24.0, 0.5), ControlWidth::Compact);
    assert_eq!(snapping.parse("15.3"), Ok(Value::Number(15.5)));
    assert_eq!(plain.parse("15.3"), Ok(Value::Number(15.3)));
}
