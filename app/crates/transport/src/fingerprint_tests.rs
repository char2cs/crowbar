use super::*;

#[test]
fn the_fingerprint_has_the_go_side_format() {
    let fingerprint = fingerprint_of(b"anything");
    let groups: Vec<&str> = fingerprint
        .strip_prefix("SHA256:")
        .unwrap()
        .split(' ')
        .collect();
    assert_eq!(groups.len(), 6);
    for group in groups {
        assert_eq!(group.len(), 4);
        assert!(
            group
                .chars()
                .all(|c| c.is_ascii_hexdigit() && !c.is_ascii_uppercase())
        );
    }
}

#[test]
fn the_fingerprint_is_deterministic_and_input_sensitive() {
    assert_eq!(fingerprint_of(b"a"), fingerprint_of(b"a"));
    assert_ne!(fingerprint_of(b"a"), fingerprint_of(b"b"));
}
