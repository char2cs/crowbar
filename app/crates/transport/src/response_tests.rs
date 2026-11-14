use super::*;

#[test]
fn only_the_proxys_own_answers_are_granted_to_the_page() {
    let page = HeaderValue::from_static("crowbar-pane://app");
    let own = readable_by(
        Some(page.clone()),
        marked_response(503, HOST_UNREACHABLE, "down"),
    );
    assert_eq!(
        own.headers().get(header::ACCESS_CONTROL_ALLOW_ORIGIN),
        Some(&page)
    );
    assert_eq!(own.headers().get(PROXY_HEADER).unwrap(), HOST_UNREACHABLE);

    let daemon = readable_by(Some(page), Response::new(Vec::new()));
    assert!(
        daemon
            .headers()
            .get(header::ACCESS_CONTROL_ALLOW_ORIGIN)
            .is_none()
    );
}

#[test]
fn no_origin_means_no_cors_headers() {
    let answer = readable_by(None, error_response(502, "x"));
    assert!(
        answer
            .headers()
            .get(header::ACCESS_CONTROL_ALLOW_ORIGIN)
            .is_none()
    );
}

#[test]
fn an_unrepresentable_status_becomes_a_bad_gateway() {
    assert_eq!(error_response(99, "x").status(), StatusCode::BAD_GATEWAY);
}
