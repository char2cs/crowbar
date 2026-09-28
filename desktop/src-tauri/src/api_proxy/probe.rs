//! First contact with a remote host — the ONE unpinned TLS dial in this app.
//! It reads the presented certificate's fingerprint for the user to confirm
//! against `crowbar pair`, then hangs up; every dial that carries data is pinned.

use std::sync::Arc;
use std::time::Duration;

use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::{
    aws_lc_rs, verify_tls12_signature, verify_tls13_signature, WebPkiSupportedAlgorithms,
};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{DigitallySignedStruct, Error as TlsError, SignatureScheme};
use tokio::io::AsyncWriteExt;
use tokio::net::TcpStream;
use tokio_rustls::TlsConnector;

use crate::tls_pin::fingerprint_of;

/// Sent as SNI only, the same name the pinned dial sends.
const SERVER_NAME: &str = "crowbar-remote";

/// Bound on connect, handshake and hang-up; the user is waiting on a dialog.
const PROBE_TIMEOUT: Duration = Duration::from_secs(5);

/// The fingerprint of the certificate `endpoint` presents, for the user to
/// confirm before `pair_remote_host` pins it. Takes no host or token state.
#[tauri::command]
pub async fn probe_remote_fingerprint(endpoint: String) -> Result<String, String> {
    tokio::time::timeout(PROBE_TIMEOUT, read_fingerprint(&endpoint))
        .await
        .unwrap_or_else(|_| {
            Err(format!(
                "{endpoint} did not answer within {PROBE_TIMEOUT:?}"
            ))
        })
}

async fn read_fingerprint(endpoint: &str) -> Result<String, String> {
    let tcp = TcpStream::connect(endpoint)
        .await
        .map_err(|e| format!("cannot reach {endpoint}: {e}"))?;
    let server_name = ServerName::try_from(SERVER_NAME).expect("SERVER_NAME is a DNS name");
    let mut tls = TlsConnector::from(unpinned_probe_config())
        .connect(server_name, tcp)
        .await
        .map_err(|e| format!("TLS handshake with {endpoint} failed: {e}"))?;
    let fingerprint = tls
        .get_ref()
        .1
        .peer_certificates()
        .and_then(|chain| chain.first())
        .map(|cert| fingerprint_of(cert))
        .ok_or_else(|| format!("{endpoint} presented no certificate"))?;
    // Best effort: the fingerprint is already proven; this only says goodbye.
    let _ = tls.shutdown().await;
    Ok(fingerprint)
}

/// `tls_pin::pinned_client_config` with one difference: no pin. Private, so
/// nothing outside this module can dial with it.
fn unpinned_probe_config() -> Arc<rustls::ClientConfig> {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let verifier = Arc::new(UnpinnedProbeVerifier {
        algorithms: provider.signature_verification_algorithms,
    });
    Arc::new(
        rustls::ClientConfig::builder_with_provider(provider)
            .with_protocol_versions(&[&rustls::version::TLS13])
            .expect("aws-lc-rs's default provider has TLS 1.3 cipher suites")
            .dangerous()
            .with_custom_certificate_verifier(verifier)
            .with_no_client_auth(),
    )
}

/// DANGER: trusts ANY certificate — anywhere but first contact, a MITM hole.
/// Sound here only because nothing is sent and a human compares the result
/// with `crowbar pair`. Signatures are still checked: the host holds the key.
#[derive(Debug)]
struct UnpinnedProbeVerifier {
    algorithms: WebPkiSupportedAlgorithms,
}

impl ServerCertVerifier for UnpinnedProbeVerifier {
    fn verify_server_cert(
        &self,
        _end_entity: &CertificateDer<'_>,
        _intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, TlsError> {
        Ok(ServerCertVerified::assertion())
    }

    fn verify_tls12_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        verify_tls12_signature(message, cert, dss, &self.algorithms)
    }

    fn verify_tls13_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        verify_tls13_signature(message, cert, dss, &self.algorithms)
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        // Only what the pinned client accepts, so any host probed is one pairing can reach.
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    }
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::super::test_support::{
        identity, refused_endpoint, serve_once_tls, serve_once_tls_speaking, unrelated_key,
    };
    use super::probe_remote_fingerprint;
    use crate::tls_pin::fingerprint_of;

    #[tokio::test]
    async fn probing_returns_the_fingerprint_of_whatever_certificate_the_host_presents() {
        let (cert, key) = identity();
        let (addr, _reported) = serve_once_tls(cert.clone(), key, None).await;

        let fingerprint = probe_remote_fingerprint(addr.to_string()).await;

        assert_eq!(fingerprint, Ok(fingerprint_of(&cert)));
    }

    #[tokio::test]
    async fn a_probe_hangs_up_cleanly_without_sending_a_request() {
        let (cert, key) = identity();
        let (addr, reported) = serve_once_tls(cert, key, None).await;

        probe_remote_fingerprint(addr.to_string())
            .await
            .expect("the probe reads the fingerprint");

        let served = reported.await.expect("server reported");
        assert_eq!(
            served,
            Ok(String::new()),
            "the unpinned channel carried data"
        );
    }

    #[tokio::test]
    async fn a_host_replaying_a_certificate_without_its_key_yields_no_fingerprint() {
        let (cert, _) = identity();
        let (addr, reported) = serve_once_tls(cert, unrelated_key(), None).await;

        let result = probe_remote_fingerprint(addr.to_string()).await;

        let err = result.expect_err("a host that cannot sign for its certificate has none to show");
        assert!(err.contains("TLS handshake"), "{err}");
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the replay completed a handshake: {served:?}"
        );
    }

    #[tokio::test]
    async fn a_host_offering_only_tls12_is_refused_as_the_pinned_client_refuses_it() {
        let (cert, key) = identity();
        let (addr, _reported) =
            serve_once_tls_speaking(&[&rustls::version::TLS12], cert, key, None).await;

        let result = probe_remote_fingerprint(addr.to_string()).await;

        let err = result.expect_err("pairing could never reach a TLS 1.2 host");
        assert!(err.contains("TLS handshake"), "{err}");
    }

    #[tokio::test]
    async fn probing_an_endpoint_nothing_listens_on_is_an_error() {
        let endpoint = refused_endpoint();

        let result = probe_remote_fingerprint(endpoint.clone()).await;

        let err = result.expect_err("nothing answered");
        assert!(err.contains(&format!("cannot reach {endpoint}")), "{err}");
    }

    #[tokio::test(start_paused = true)]
    async fn a_host_that_never_completes_the_handshake_fails_at_the_probe_deadline() {
        // Listening, so the kernel completes the TCP handshake, but never speaks TLS.
        let silent = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let endpoint = silent.local_addr().unwrap().to_string();

        let probed =
            tokio::time::timeout(Duration::from_secs(60), probe_remote_fingerprint(endpoint));
        let result = probed
            .await
            .expect("must fail at the probe deadline, not hang");

        let err = result.expect_err("a silent host has no fingerprint");
        assert!(err.contains("did not answer within"), "{err}");
    }
}
