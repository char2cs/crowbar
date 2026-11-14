use super::*;

#[test]
fn accepts_absolute_paths_with_queries() {
    for ok in ["/", "/v0/health", "/v0/threads?limit=5&q=a%20b"] {
        assert!(RequestTarget::new(ok).is_ok(), "{ok}");
    }
}

#[test]
fn refuses_anything_that_could_name_another_host_or_split_the_request() {
    for bad in [
        "",
        "v0/x",
        "@host-2/v0/x",
        "-x/v0/x",
        ":80/v0/x",
        ".evil/v0/x",
        "\\\\host\\x",
        "//host-2/v0/x",
        "/v0/a b",
        "/v0/a\r\nHost: x",
        "/v0/\u{e9}",
        "/v0/x#frag",
        "/v0\\x",
    ] {
        assert!(RequestTarget::new(bad).is_err(), "{bad:?}");
    }
    assert!(
        RequestTarget::new(format!("/{}", "a".repeat(MAX_TARGET_BYTES))).is_err(),
        "too long"
    );
}
