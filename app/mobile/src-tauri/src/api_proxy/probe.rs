use std::sync::Arc;
use std::time::Duration;

use rustls::crypto::aws_lc_rs;
use tokio::io::AsyncWriteExt;
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;
use tokio_rustls::TlsConnector;

use super::remote::server_name;
use crate::hosts::endpoint::{parse_endpoint, Endpoint};
use crate::tls_pin::fingerprint_of;
use unpinned_probe_verifier::UnpinnedProbeVerifier;

mod unpinned_probe_verifier;

const PROBE_TIMEOUT: Duration = Duration::from_secs(5);

#[tauri::command]
pub(crate) async fn probe_remote_fingerprint(endpoint: String) -> Result<String, String> {
    let endpoint = parse_endpoint(&endpoint)?;
    tokio::time::timeout(PROBE_TIMEOUT, read_fingerprint(&endpoint))
        .await
        .unwrap_or_else(|_| {
            Err(format!(
                "{endpoint} did not answer within {PROBE_TIMEOUT:?}"
            ))
        })
}

async fn read_fingerprint(endpoint: &Endpoint) -> Result<String, String> {
    let tcp = endpoint.connect().await?;
    let mut tls = TlsConnector::from(unpinned_probe_config()?)
        .connect(server_name()?, tcp)
        .await
        .map_err(|e| format!("TLS handshake with {endpoint} failed: {e}"))?;
    let fingerprint = presented_fingerprint(&tls)
        .ok_or_else(|| format!("{endpoint} presented no certificate"))?;
    if let Err(error) = tls.shutdown().await {
        log::debug!("probe: hang-up after reading the fingerprint failed: {error}");
    }
    Ok(fingerprint)
}

fn presented_fingerprint(tls: &TlsStream<TcpStream>) -> Option<String> {
    let chain = tls.get_ref().1.peer_certificates()?;
    chain.first().map(|cert| fingerprint_of(cert))
}

fn unpinned_probe_config() -> Result<Arc<rustls::ClientConfig>, String> {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let verifier = Arc::new(UnpinnedProbeVerifier {
        algorithms: provider.signature_verification_algorithms,
    });
    let config = rustls::ClientConfig::builder_with_provider(provider)
        .with_protocol_versions(&[&rustls::version::TLS13])
        .map_err(|e| format!("TLS 1.3 is unavailable: {e}"))?
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_no_client_auth();
    Ok(Arc::new(config))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::proxy_fakes::{
        identity, refused_endpoint, serve_once_tls, serve_once_tls_speaking, unrelated_key,
    };

    #[tokio::test]
    async fn probing_returns_the_fingerprint_of_whatever_certificate_the_host_presents() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (addr, _reported) = serve_once_tls(cert.clone(), key, None).await;

        let by_address = probe_remote_fingerprint(addr.to_string()).await;

        assert_eq!(by_address, Ok(fingerprint_of(&cert)));
    }

    #[tokio::test]
    async fn a_name_that_does_not_resolve_is_told_apart_from_a_host_that_does_not_answer() {
        let _serialised = crate::test_support::fd_tests().await;
        let refused = refused_endpoint();

        let unresolved = probe_remote_fingerprint("nowhere.invalid:3737".into())
            .await
            .expect_err("no such name");
        let silent = probe_remote_fingerprint(refused.clone())
            .await
            .expect_err("nothing answered");

        assert!(
            unresolved.starts_with("cannot resolve nowhere.invalid"),
            "{unresolved}"
        );
        assert!(
            silent.contains(&format!("cannot reach {refused}")),
            "{silent}"
        );
    }

    #[tokio::test]
    async fn an_address_that_is_not_host_and_port_is_refused_before_anything_is_dialed() {
        an_address_that_is_not_host_and_port_is_run().await;
    }

    async fn an_address_that_is_not_host_and_port_is_run() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (addr, mut reported) = serve_once_tls(cert, key, None).await;
        let port = addr.port();
        let cases = [
            format!("https://127.0.0.1:{port}"),
            format!("127.0.0.1:{port}/"),
            format!("localhost:{port}/v0"),
            format!("me@localhost:{port}"),
            "localhost".to_owned(),
        ];

        for endpoint in cases {
            let err = probe_remote_fingerprint(endpoint.clone())
                .await
                .expect_err(&endpoint);
            assert!(err.contains("is not a host:port address"), "{err}");
        }
        assert!(reported.try_recv().is_err(), "the host was dialed");
    }

    #[tokio::test]
    async fn a_probe_hangs_up_cleanly_without_sending_a_request() {
        let _serialised = crate::test_support::fd_tests().await;
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
    async fn a_host_that_cannot_complete_a_tls_13_handshake_yields_no_fingerprint() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (replay, replayed) = serve_once_tls(cert.clone(), unrelated_key(), None).await;
        let tls12 = [&rustls::version::TLS12];
        let (old, _old_reported) = serve_once_tls_speaking(&tls12, cert, key, None).await;

        let without_key = probe_remote_fingerprint(replay.to_string()).await;
        let tls12_only = probe_remote_fingerprint(old.to_string()).await;

        let err = without_key.expect_err("a host that cannot sign has no certificate to show");
        assert!(err.contains("TLS handshake"), "{err}");
        assert!(
            replayed.await.unwrap().is_err(),
            "the replay completed a handshake"
        );
        let err = tls12_only.expect_err("pairing could never reach a TLS 1.2 host");
        assert!(err.contains("TLS handshake"), "{err}");
    }

    #[tokio::test(start_paused = true)]
    async fn a_host_that_never_completes_the_handshake_fails_at_the_probe_deadline() {
        let _serialised = crate::test_support::fd_tests().await;
        let silent = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let endpoint = silent.local_addr().unwrap().to_string();

        let probed =
            tokio::time::timeout(Duration::from_secs(60), probe_remote_fingerprint(endpoint));
        let result = probed.await.expect("must fail at the probe deadline");

        let err = result.expect_err("a silent host has no fingerprint");
        assert!(err.contains("did not answer within"), "{err}");
    }

    #[test]
    fn the_unpinned_config_builds_for_tls_13() {
        assert!(unpinned_probe_config().is_ok(), "config must build");
    }
}
