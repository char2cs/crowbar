use super::*;

#[test]
fn wire_names_match_serde() {
    for event in [
        Lifecycle::Mount,
        Lifecycle::Activate,
        Lifecycle::Deactivate,
        Lifecycle::Park,
        Lifecycle::Destroy,
    ] {
        let json = serde_json::to_string(&event).unwrap();
        assert_eq!(json, format!("\"{}\"", event.as_str()), "{event:?}");
    }
}
