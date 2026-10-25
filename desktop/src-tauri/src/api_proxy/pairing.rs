//! Pairing a remote host: a one-time code from `crowbar pair` is redeemed for a
//! bearer token over the pinned channel, and the token goes straight into the
//! keychain, bound to the certificate that channel verified — it never crosses
//! into the webview.

use std::sync::Arc;
use std::time::Duration;

use tauri::{http, State};
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;

use super::proxy;
use super::remote::{dial, RemoteHost, REMOTE_BODY_LIMIT};
use crate::host_tokens::{Credential, TokenStore};
use crate::hosts::endpoint::parse_endpoint;
use crate::hosts::{validate_host_id, HostRegistry};
use crate::tls_pin::{certificate_digest, CertificateDigest, Pin};

/// Bound on the whole redeem: dial, request and answer.
const PAIR_TIMEOUT: Duration = Duration::from_secs(30);

/// Redeems `code` with the daemon at `endpoint`, trusting only the certificate
/// matching `fingerprint`. Only a stored credential registers the host; on any
/// failure the registry and keychain are left as they were. Resolves to the
/// daemon's id for this client — not a secret, and what a server-side revoke
/// (`DELETE /v0/clients/:id`) names.
#[tauri::command]
pub async fn pair_remote_host(
    host_id: String,
    endpoint: String,
    fingerprint: String,
    code: String,
    name: String,
    registry: State<'_, HostRegistry>,
    tokens: State<'_, Arc<dyn TokenStore>>,
) -> Result<String, String> {
    let host = RemoteHost {
        endpoint: parse_endpoint(&endpoint)?,
        pin: Pin::Confirmed(fingerprint),
    };
    pair(&registry, &tokens, &host_id, host, &code, &name).await
}

async fn pair(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
    host: RemoteHost,
    code: &str,
    name: &str,
) -> Result<String, String> {
    // Before dialing: the code is single-use, and no registry would take this id.
    validate_host_id(host_id)?;
    let redeemed = tokio::time::timeout(PAIR_TIMEOUT, redeem(&host, code, name))
        .await
        .map_err(|_| {
            format!(
                "{} did not finish pairing within {PAIR_TIMEOUT:?}",
                host.endpoint
            )
        })??;
    keep_credential(tokens, host_id, &redeemed.credential).await?;
    registry.insert(host_id.to_string(), host.endpoint)?;
    Ok(redeemed.client_id)
}

struct Redeemed {
    credential: Credential,
    client_id: String,
}

async fn redeem(host: &RemoteHost, code: &str, name: &str) -> Result<Redeemed, String> {
    let tls = dial(host).await.map_err(|e| e.to_string())?;
    // Later dials pin this whole digest, not the 96 bits a human compared.
    let pin =
        peer_digest(&tls).ok_or_else(|| format!("{} presented no certificate", host.endpoint))?;
    let body = serde_json::json!({ "code": code, "name": name }).to_string();
    let request = http::Request::builder()
        .method(http::Method::POST)
        .uri("/v0/pair")
        .header(http::header::CONTENT_TYPE, "application/json")
        .body(body.into_bytes())
        .map_err(|e| e.to_string())?;
    let resp = proxy(tls, request, REMOTE_BODY_LIMIT)
        .await
        .map_err(|e| format!("pairing with {} failed: {e}", host.endpoint))?;
    let (token, client_id) = pairing_answer(&resp)?;
    Ok(Redeemed {
        credential: Credential { token, pin },
        client_id,
    })
}

fn peer_digest(tls: &TlsStream<TcpStream>) -> Option<CertificateDigest> {
    let chain = tls.get_ref().1.peer_certificates()?;
    chain.first().map(|cert| certificate_digest(cert))
}

/// `data.token` and `data.clientId` from the daemon's envelope, or its `error`
/// when it refused.
fn pairing_answer(resp: &http::Response<Vec<u8>>) -> Result<(String, String), String> {
    let body: serde_json::Value = serde_json::from_slice(resp.body()).unwrap_or_default();
    if !resp.status().is_success() {
        let reason = body["error"].as_str().unwrap_or("no reason given");
        return Err(format!(
            "the host refused pairing ({}): {reason}",
            resp.status()
        ));
    }
    let field = |name: &str| {
        body["data"][name]
            .as_str()
            .filter(|value| !value.trim().is_empty())
            .map(str::to_string)
            .ok_or_else(|| format!("the host's pairing answer carried no {name}"))
    };
    Ok((field("token")?, field("clientId")?))
}

/// The keychain write can block on an OS prompt, so it runs on the blocking pool.
async fn keep_credential(
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
    credential: &Credential,
) -> Result<(), String> {
    let tokens = Arc::clone(tokens);
    let host_id = host_id.to_string();
    let entry = credential.encode();
    tokio::task::spawn_blocking(move || tokens.set(&host_id, &entry))
        .await
        .map_err(|e| e.to_string())?
}

#[cfg(test)]
mod tests {
    use std::net::SocketAddr;
    use std::time::Duration;

    use serde_json::{json, Value};

    use super::super::test_support::{
        header, identity, registry, response, serve_once_tls, Keychain, HOST_ID,
    };
    use super::pair;
    use crate::api_proxy::remote::RemoteHost;
    use crate::host_tokens::Credential;
    use crate::hosts::endpoint::parse_endpoint;
    use crate::hosts::HostRegistry;
    use crate::tls_pin::{certificate_digest, fingerprint_of, Pin};

    const PAIRED: &str = r#"{"success":true,"data":{"token":"minted-token","clientId":"c1"}}"#;

    fn host(addr: SocketAddr, fingerprint: String) -> RemoteHost {
        host_at(&addr.to_string(), fingerprint)
    }

    fn host_at(endpoint: &str, fingerprint: String) -> RemoteHost {
        RemoteHost {
            endpoint: parse_endpoint(endpoint).unwrap(),
            pin: Pin::Confirmed(fingerprint),
        }
    }

    fn registered(registry: &HostRegistry) -> Option<String> {
        registry.get(HOST_ID).map(|endpoint| endpoint.to_string())
    }

    #[tokio::test]
    async fn pairing_redeems_the_code_over_the_pinned_channel_and_keeps_the_token() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let reply = Some(response("200 OK", PAIRED));
        let (addr, reported) = serve_once_tls(cert.clone(), key, reply).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let client_id = pair(
            &registry,
            &keychain,
            HOST_ID,
            host(addr, fingerprint),
            "123456",
            "laptop",
        )
        .await
        .expect("pairing succeeds");

        assert_eq!(client_id, "c1", "the id a server-side revoke names");
        let stored = keychain
            .get(HOST_ID)
            .unwrap()
            .expect("a credential is kept");
        assert_eq!(
            Credential::decode(&stored),
            Some(Credential {
                token: "minted-token".into(),
                pin: certificate_digest(&cert),
            }),
            "the token must be bound to the whole digest of the certificate it came from"
        );
        assert_eq!(registered(&registry), Some(addr.to_string()));
        let request = reported
            .await
            .expect("server reported")
            .expect("handshake with the pinned host");
        assert!(
            request.starts_with("POST /v0/pair HTTP/1.1\r\n"),
            "{request}"
        );
        assert_eq!(header(&request, "content-type"), Some("application/json"));
        assert_eq!(header(&request, "authorization"), None);
        let body: Value = serde_json::from_str(request.split_once("\r\n\r\n").unwrap().1).unwrap();
        assert_eq!(body, json!({"code": "123456", "name": "laptop"}));
    }

    // The pinned dial resolves a name as the probe does; trust stays the pinned certificate.
    #[tokio::test]
    async fn a_host_paired_by_name_is_dialed_and_registered_by_that_name() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let reply = Some(response("200 OK", PAIRED));
        let (addr, reported) = serve_once_tls(cert, key, reply).await;
        let endpoint = format!("localhost:{}", addr.port());
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let host = host_at(&endpoint, fingerprint);
        pair(&registry, &keychain, HOST_ID, host, "123456", "laptop")
            .await
            .expect("pairing by name succeeds");

        assert_eq!(registered(&registry), Some(endpoint));
        let request = reported.await.unwrap().expect("handshake by name");
        assert!(request.starts_with("POST /v0/pair "), "{request}");
    }

    // Dialed by name: wherever a name points, only the confirmed certificate is trusted.
    #[tokio::test]
    async fn pairing_with_a_host_presenting_another_certificate_sends_and_keeps_nothing() {
        let _serialised = crate::test_support::fd_tests().await;
        let (pinned, _) = identity();
        let (impostor_cert, impostor_key) = identity();
        let reply = Some(response("200 OK", PAIRED));
        let (addr, reported) = serve_once_tls(impostor_cert, impostor_key, reply).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let impostor = host_at(
            &format!("localhost:{}", addr.port()),
            fingerprint_of(&pinned),
        );
        let err = pair(&registry, &keychain, HOST_ID, impostor, "123456", "laptop")
            .await
            .expect_err("an impostor must not be paired");

        assert!(err.contains("identity changed"), "{err}");
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the impostor read the pairing code: {served:?}"
        );
        assert_eq!(keychain.get(HOST_ID).unwrap(), None);
        assert!(registry.get(HOST_ID).is_none());
    }

    #[tokio::test]
    async fn a_refused_code_leaves_an_existing_pairing_as_it_was() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let refused = r#"{"success":false,"error":"pairing code is not valid"}"#;
        let reply = Some(response("401 Unauthorized", refused));
        let (addr, _reported) = serve_once_tls(cert, key, reply).await;
        let registry = registry("10.0.0.1:3737".into());
        let keychain = Keychain::holding(&[(HOST_ID, "previous-credential")]);

        let err = pair(
            &registry,
            &keychain,
            HOST_ID,
            host(addr, fingerprint),
            "000000",
            "laptop",
        )
        .await
        .expect_err("a refused code fails");

        assert!(err.contains("pairing code is not valid"), "{err}");
        assert_eq!(
            keychain.get(HOST_ID).unwrap().as_deref(),
            Some("previous-credential")
        );
        assert_eq!(registered(&registry).as_deref(), Some("10.0.0.1:3737"));
    }

    #[tokio::test]
    async fn an_answer_without_a_client_id_pairs_nothing() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let tokenonly = r#"{"success":true,"data":{"token":"minted-token"}}"#;
        let (addr, _reported) =
            serve_once_tls(cert, key, Some(response("200 OK", tokenonly))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let err = pair(
            &registry,
            &keychain,
            HOST_ID,
            host(addr, fingerprint),
            "123456",
            "laptop",
        )
        .await
        .expect_err("a pairing nothing could revoke is refused");

        assert!(err.contains("no clientId"), "{err}");
        assert_eq!(keychain.get(HOST_ID).unwrap(), None);
        assert!(registry.get(HOST_ID).is_none());
    }

    #[tokio::test]
    async fn a_token_the_keychain_refuses_leaves_the_host_unregistered() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, _reported) = serve_once_tls(cert, key, Some(response("200 OK", PAIRED))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::broken("keychain is locked");

        let err = pair(
            &registry,
            &keychain,
            HOST_ID,
            host(addr, fingerprint),
            "123456",
            "laptop",
        )
        .await
        .expect_err("an unstorable token fails the pairing");

        assert!(err.contains("keychain is locked"), "{err}");
        assert!(registry.get(HOST_ID).is_none());
    }

    #[tokio::test]
    async fn a_host_id_the_router_would_not_send_to_that_host_spends_no_code() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let reply = Some(response("200 OK", PAIRED));
        let (addr, mut reported) = serve_once_tls(cert, key, reply).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let err = pair(
            &registry,
            &keychain,
            "localhost",
            host(addr, fingerprint),
            "123456",
            "laptop",
        )
        .await
        .expect_err("the local daemon's name is no remote host's");

        assert!(err.contains("host id"), "{err}");
        assert!(
            reported.try_recv().is_err(),
            "the host was dialed: {:?}",
            reported.try_recv()
        );
        assert_eq!(keychain.get("localhost").unwrap(), None);
    }

    #[tokio::test]
    async fn pairing_with_a_host_that_never_answers_fails_at_the_deadline() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, reported) = serve_once_tls(cert, key, None).await;
        let pairing = tokio::spawn(async move {
            let registry = HostRegistry::default();
            let keychain = Keychain::holding(&[]);
            let paired = pair(
                &registry,
                &keychain,
                HOST_ID,
                host(addr, fingerprint),
                "123456",
                "laptop",
            )
            .await;
            (paired, registry.get(HOST_ID), keychain.get(HOST_ID))
        });

        // Frozen only once the request is in, so the handshake runs on real time.
        reported
            .await
            .unwrap()
            .expect("handshake with the pinned host");
        tokio::time::pause();
        let (paired, registered, kept) = tokio::time::timeout(Duration::from_secs(600), pairing)
            .await
            .expect("must fail at the pairing deadline, not hang")
            .unwrap();

        let err = paired.expect_err("a silent host is not paired");
        assert!(err.contains("did not finish pairing"), "{err}");
        assert_eq!(kept, Ok(None));
        assert!(registered.is_none());
    }
}
