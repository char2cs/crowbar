use super::*;

#[test]
fn round_trips_and_validates_both_parts() {
    let binding = KeyBinding::new(
        KeyChord::new("mod+k").unwrap(),
        CommandId::new("core.palette").unwrap(),
    );
    let text = serde_json::to_string(&binding).unwrap();
    assert_eq!(
        text, r#"{"keys":"mod+k","command":"core.palette"}"#,
        "wire shape"
    );
    assert_eq!(
        serde_json::from_str::<KeyBinding>(&text).unwrap(),
        binding,
        "round trip"
    );
    assert_eq!(binding.keys().as_str(), "mod+k", "keys");
    assert_eq!(binding.command().as_str(), "core.palette", "command");
    assert!(
        serde_json::from_str::<KeyBinding>(r#"{"keys":"mod k","command":"x"}"#).is_err(),
        "bad chord"
    );
    assert!(
        serde_json::from_str::<KeyBinding>(r#"{"keys":"k","command":"a b"}"#).is_err(),
        "bad command"
    );
}
