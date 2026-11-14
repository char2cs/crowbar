use super::*;
use crate::extension_id::ExtensionId;

fn request() -> DaemonRequest {
    DaemonRequest::new(
        HostId::new("local").unwrap(),
        PaneOrigin::Extension(ExtensionId::new("git").unwrap()),
        HttpMethod::new("POST").unwrap(),
        RequestTarget::new("/v0/ext/git/x?a=1").unwrap(),
    )
}

#[test]
fn getters_return_what_was_built() {
    let built = request()
        .with_headers(vec![("x-a".into(), "1".into())])
        .unwrap()
        .with_body(b"body".to_vec());
    assert_eq!(built.host().as_str(), "local", "host");
    assert_eq!(
        built.origin().extension().map(ExtensionId::as_str),
        Some("git"),
        "origin"
    );
    assert_eq!(built.method().as_str(), "POST", "method");
    assert_eq!(built.target().as_str(), "/v0/ext/git/x?a=1", "target");
    assert_eq!(
        built.headers(),
        [("x-a".to_owned(), "1".to_owned())],
        "headers"
    );
    assert_eq!(built.body(), b"body", "body");
    assert_eq!(built.into_body(), b"body".to_vec(), "moved body");
}

#[test]
fn header_count_and_size_are_capped() {
    let many = vec![("x".to_owned(), "v".to_owned()); MAX_HEADERS + 1];
    assert!(request().with_headers(many).is_err(), "too many");
    let big = vec![("x".to_owned(), "v".repeat(MAX_HEADER_BYTES))];
    assert!(request().with_headers(big).is_err(), "too large");
    let at_cap = vec![("x".to_owned(), "v".to_owned()); MAX_HEADERS];
    assert!(request().with_headers(at_cap).is_ok(), "at the cap");
}
