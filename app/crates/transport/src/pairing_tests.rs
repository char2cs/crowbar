use super::*;

fn answer(status: u16, body: &str) -> Response<Vec<u8>> {
    let mut response = Response::new(body.as_bytes().to_vec());
    *response.status_mut() = http::StatusCode::from_u16(status).unwrap();
    response
}

#[test]
fn a_pairing_answer_yields_its_token_and_client_id() {
    let ok = answer(200, r#"{"data":{"token":"t","clientId":"c1"}}"#);
    assert_eq!(pairing_answer(&ok).unwrap(), ("t".into(), "c1".into()));
}

#[test]
fn a_refusal_carries_the_hosts_reason() {
    let refused = answer(403, r#"{"error":"code expired"}"#);
    let err = pairing_answer(&refused).unwrap_err();
    assert!(err.to_string().contains("code expired"), "{err}");
}

#[test]
fn an_answer_without_a_token_or_client_id_is_refused() {
    for body in [
        r#"{"data":{"clientId":"c"}}"#,
        r#"{"data":{"token":" ","clientId":"c"}}"#,
        r#"{"data":{"token":"t"}}"#,
        "nonsense",
    ] {
        assert!(
            matches!(
                pairing_answer(&answer(200, body)),
                Err(Error::MissingField(_))
            ),
            "{body}"
        );
    }
}
