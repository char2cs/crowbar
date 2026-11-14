use super::*;

#[test]
fn builds_serializes_and_looks_up_tokens() {
    let tokens = ThemeTokens::default()
        .with("color-bg", "#1a1b1e")
        .unwrap()
        .with("radius.sm", "6px")
        .unwrap();
    assert_eq!(
        serde_json::to_string(&tokens).unwrap(),
        r##"{"color-bg":"#1a1b1e","radius.sm":"6px"}"##,
        "shape"
    );
    assert_eq!(tokens.get("radius.sm"), Some("6px"), "lookup");
    assert_eq!((tokens.len(), tokens.is_empty()), (2, false), "size");
    assert!(ThemeTokens::default().is_empty(), "empty");
}

#[test]
fn names_values_and_count_are_bounded_also_on_the_wire() {
    for (name, value) in [
        ("", "v"),
        ("Bad", "v"),
        ("a b", "v"),
        ("a", "x\ny"),
        ("a", "x".repeat(129).as_str()),
    ] {
        assert!(
            ThemeTokens::default().with(name, value).is_err(),
            "{name:?}={value:?}"
        );
    }
    let wire: serde_json::Map<String, serde_json::Value> = (0..=MAX_THEME_TOKENS)
        .map(|index| (format!("t{index}"), "v".into()))
        .collect();
    assert!(
        serde_json::from_value::<ThemeTokens>(wire.into()).is_err(),
        "too many on the wire"
    );
    assert!(
        serde_json::from_str::<ThemeTokens>(r#"{"A":"v"}"#).is_err(),
        "bad name on the wire"
    );
}
