use super::*;

#[test]
fn bounds_become_logical_rects_with_a_minimum_size() {
    let rect = to_rect(Bounds {
        x: 1.0,
        y: 2.0,
        width: 0.0,
        height: 30.0,
    });
    assert_eq!(
        rect.size.to_logical::<f64>(1.0).width,
        1.0,
        "zero width is lifted to one pixel"
    );
    assert_eq!(rect.position.to_logical::<f64>(1.0).y, 2.0, "position kept");
}

#[test]
fn protocol_responses_keep_status_headers_and_body() {
    let response = into_http(ProtocolResponse {
        status: 201,
        headers: vec![("content-type".into(), "text/plain".into())],
        body: b"hi".to_vec(),
    });
    assert_eq!(response.status().as_u16(), 201, "status");
    assert_eq!(
        response.headers().get("content-type").map(|v| v.as_bytes()),
        Some(&b"text/plain"[..]),
        "header"
    );
    assert_eq!(response.body().as_ref(), b"hi", "body");
}

#[test]
fn invalid_headers_turn_into_a_500() {
    let response = into_http(ProtocolResponse {
        status: 200,
        headers: vec![("bad header".into(), "x".into())],
        body: Vec::new(),
    });
    assert_eq!(response.status().as_u16(), 500, "status");
}
