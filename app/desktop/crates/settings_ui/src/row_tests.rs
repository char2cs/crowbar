use super::*;

#[test]
fn builders_fill_the_row() {
    let row = Row::new("tabSize", "Tab Size", Control::switch())
        .describe("Spaces per tab")
        .describe_invalid("Bad")
        .default_value(2.0)
        .when(Condition::capability("editor"))
        .keywords(&["indent", "spaces"]);
    assert_eq!((row.key().as_str(), row.label()), ("tabSize", "Tab Size"));
    assert_eq!(row.description.as_deref(), Some("Spaces per tab"));
    assert_eq!(row.invalid_description.as_deref(), Some("Bad"));
    assert_eq!(row.default, Some(Value::Number(2.0)));
    assert!(row.when.is_some());
    assert_eq!(row.keywords, ["indent", "spaces"]);
}
