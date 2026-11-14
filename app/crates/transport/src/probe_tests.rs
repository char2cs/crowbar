use super::*;

#[tokio::test]
async fn an_address_that_is_not_host_and_port_is_refused_before_anything_is_dialed() {
    for text in ["", "example.com", "https://example.com:443", "1.2.3.4:0"] {
        assert!(
            matches!(
                probe_fingerprint(text).await,
                Err(Error::MalformedEndpoint { .. })
            ),
            "{text}"
        );
    }
}
