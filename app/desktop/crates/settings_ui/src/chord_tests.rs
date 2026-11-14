use super::*;

#[test]
fn parse_normalises_order_and_case() {
    const CASES: &[(&str, Option<&str>)] = &[
        ("Mod+Shift+T", Some("mod+shift+t")),
        ("alt+mod+n", Some("mod+alt+n")),
        ("shift+alt+mod+arrowLeft", Some("mod+shift+alt+arrowleft")),
        ("mod+`", Some("mod+`")),
        ("mod++", Some("mod++")),
        ("f5", Some("f5")),
        ("mod+mod+t", None),
        ("mod+", None),
        ("", None),
        ("shift", None),
    ];
    for &(text, expected) in CASES {
        assert_eq!(
            Chord::parse(text).as_ref().map(Chord::as_str),
            expected,
            "{text:?}"
        );
    }
}

#[test]
fn labels_follow_the_platform() {
    let chord = Chord::parse("mod+shift+alt+arrowleft").unwrap();
    assert_eq!(chord.label(Primary::Control), "Ctrl+Shift+Alt+\u{2190}");
    assert_eq!(
        chord.label(Primary::Platform),
        "\u{2318}\u{21e7}\u{2325}\u{2190}"
    );
    assert_eq!(
        Chord::parse("mod+t").unwrap().label(Primary::Control),
        "Ctrl+T"
    );
}
