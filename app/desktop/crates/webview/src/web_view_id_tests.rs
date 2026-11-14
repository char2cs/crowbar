use super::*;

#[test]
fn text_form_round_trips() {
    let id = WebViewId(42);
    assert_eq!(id.to_string(), "wv-42", "display");
    assert_eq!(WebViewId::parse("wv-42"), Some(id), "parse");
    assert_eq!(WebViewId::parse("42"), None, "missing prefix");
    assert_eq!(WebViewId::parse("wv-x"), None, "not a number");
}
