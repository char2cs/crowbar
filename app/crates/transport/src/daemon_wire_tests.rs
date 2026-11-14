use protocol::PaneKind;

use super::*;

fn request(host: &str, path: &str) -> DaemonRequest {
    DaemonRequest {
        host: HostId::new(host).unwrap(),
        origin: PaneKind::Core,
        extension: None,
        method: "POST".into(),
        path_and_query: path.into(),
        headers: vec![("content-type".into(), "application/json".into())],
        body: b"{}".to_vec(),
    }
}

#[test]
fn the_local_host_is_spelled_localhost_on_the_wire() {
    let (scope, http_request) = to_http(&request("local", "/v0/x?y=1")).unwrap();
    assert_eq!(scope, AccessScope::Core);
    assert_eq!(
        http_request.uri().to_string(),
        "crowbar://localhost/v0/x?y=1"
    );
    assert_eq!(http_request.method(), Method::POST);
    assert_eq!(http_request.headers()["content-type"], "application/json");
    let (_, remote) = to_http(&request("host-1", "/v0/x")).unwrap();
    assert_eq!(remote.uri().host(), Some("host-1"));
}

#[test]
fn a_request_that_cannot_be_spelled_is_refused_not_guessed() {
    let mut bad_method = request("local", "/v0/x");
    bad_method.method = "NOT A METHOD".into();
    let mut bad_header = request("local", "/v0/x");
    bad_header.headers = vec![("bad name".into(), "v".into())];
    let mut contradictory = request("local", "/v0/x");
    contradictory.extension = Some(protocol::ExtensionId::new("acme").unwrap());
    for case in [bad_method, bad_header, contradictory] {
        assert!(to_http(&case).is_err());
    }
}

#[test]
fn a_response_keeps_status_headers_and_body() {
    let mut answer = error_response(503, "down");
    answer
        .headers_mut()
        .insert("x-extra", HeaderValue::from_static("1"));
    let wire = from_http(&answer);
    assert_eq!(wire.status, 503);
    assert_eq!(wire.body, b"down");
    assert!(
        wire.headers
            .contains(&("x-extra".to_owned(), "1".to_owned()))
    );
    assert_eq!(refusal(&Error::BadGrant("x")).status, 403);
}
