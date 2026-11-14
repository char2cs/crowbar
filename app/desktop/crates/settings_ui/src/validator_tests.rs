use super::*;

#[test]
fn search_template_rules() {
    const CASES: &[(&str, bool)] = &[
        ("", true),
        ("https://kagi.com/search?q=%s", true),
        ("http://example.com/?q=%s", true),
        ("https://kagi.com/search", false),
        ("ftp://example.com/%s", false),
        ("https://", false),
        ("https://a b/%s", false),
        ("kagi.com/%s", false),
    ];
    for &(text, valid) in CASES {
        assert_eq!(
            Validator::SearchTemplate.check(text).is_ok(),
            valid,
            "{text:?}"
        );
    }
}
