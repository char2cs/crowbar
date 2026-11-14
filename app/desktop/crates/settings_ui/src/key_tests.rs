use super::*;

#[test]
fn namespace_membership_needs_the_dot_boundary() {
    const CASES: &[(&str, &str, bool)] = &[
        ("ext.git.autoFetch", "ext.git", true),
        ("ext.gitlab.token", "ext.git", false),
        ("ext.git", "ext.git", false),
        ("uiFontSize", "ext.git", false),
    ];
    for &(key, namespace, expected) in CASES {
        assert_eq!(
            Key::new(key).is_in(namespace),
            expected,
            "{key} in {namespace}"
        );
    }
}

#[test]
fn displays_as_its_name() {
    assert_eq!(Key::from("tabSize").to_string(), "tabSize");
    assert_eq!(Key::from("tabSize").as_str(), "tabSize");
}
