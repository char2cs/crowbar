use super::*;

#[test]
fn getters_return_what_was_built() {
    let response = DaemonResponse::new(201, vec![("x-a".into(), "1".into())], b"ok".to_vec());
    assert_eq!(response.status(), 201, "status");
    assert_eq!(
        response.headers(),
        [("x-a".to_owned(), "1".to_owned())],
        "headers"
    );
    assert_eq!(response.body(), b"ok", "body");
    assert_eq!(response.into_body(), b"ok".to_vec(), "moved body");
}
