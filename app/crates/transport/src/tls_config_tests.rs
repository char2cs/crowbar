use rustls::crypto::aws_lc_rs;

use super::*;
use crate::pin::Pin;
use crate::pinned_verifier::PinnedVerifier;

#[test]
fn a_config_is_built_for_tls13_with_a_custom_verifier() {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let verifier = Arc::new(PinnedVerifier::new(Pin::Paired([0; 32]), &provider));
    assert!(client_config_with(provider, verifier).is_ok());
}
