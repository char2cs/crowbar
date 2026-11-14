use super::*;

#[test]
fn negotiation_needs_equal_majors_and_takes_the_lower_minor() {
    let shell = SdkVersion::new(1, 2);
    assert_eq!(
        SdkVersion::new(1, 5).negotiate(shell).unwrap(),
        SdkVersion::new(1, 2),
        "newer page"
    );
    assert_eq!(
        SdkVersion::new(1, 0).negotiate(shell).unwrap(),
        SdkVersion::new(1, 0),
        "older page"
    );
    for major in [0, 2] {
        assert!(
            matches!(
                SdkVersion::new(major, 0).negotiate(shell),
                Err(Error::UnsupportedSdk { expected: 1, .. })
            ),
            "major {major}"
        );
    }
}

#[test]
fn serializes_as_major_dot_minor() {
    assert_eq!(
        serde_json::to_string(&SDK_VERSION).unwrap(),
        "\"1.0\"",
        "serialize"
    );
    assert_eq!(
        serde_json::from_str::<SdkVersion>("\"1.12\"").unwrap(),
        SdkVersion::new(1, 12),
        "parse"
    );
    assert_eq!(SdkVersion::new(1, 2).to_string(), "1.2", "display");
    assert_eq!((SDK_VERSION.major(), SDK_VERSION.minor()), (1, 0), "parts");
    for bad in [
        "1",
        "1.",
        ".1",
        "a.b",
        "1.2.3",
        "-1.0",
        "1.+2",
        "1.0000000000",
    ] {
        assert!(
            serde_json::from_str::<SdkVersion>(&format!("\"{bad}\"")).is_err(),
            "{bad}"
        );
    }
}
