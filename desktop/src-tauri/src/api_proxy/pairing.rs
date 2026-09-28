//! Pairing a remote host: a one-time code from `crowbar pair` is redeemed for a
//! bearer token over the pinned channel, and the token goes straight into the
//! keychain — it never crosses into the webview.

use std::sync::Arc;
use std::time::Duration;

use tauri::{http, State};

use super::proxy;
use super::remote::dial;
use crate::host_tokens::TokenStore;
use crate::hosts::{HostRegistry, RemoteHost};

/// Bound on the whole redeem: dial, request and answer.
const PAIR_TIMEOUT: Duration = Duration::from_secs(30);

/// Redeems `code` with the daemon at `endpoint`, trusting only the certificate
/// matching `fingerprint`. Only a stored token registers the host; on any
/// failure the registry and keychain are left as they were.
#[tauri::command]
pub async fn pair_remote_host(
    host_id: String,
    endpoint: String,
    fingerprint: String,
    code: String,
    name: String,
    registry: State<'_, HostRegistry>,
    tokens: State<'_, Arc<dyn TokenStore>>,
) -> Result<(), String> {
    let host = RemoteHost {
        endpoint,
        fingerprint,
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
) -> Result<(), String> {
    let token = tokio::time::timeout(PAIR_TIMEOUT, redeem(&host, code, name))
        .await
        .map_err(|_| {
            format!(
                "{} did not finish pairing within {PAIR_TIMEOUT:?}",
                host.endpoint
            )
        })??;
    keep_token(tokens, host_id, token).await?;
    registry.insert(host_id.to_string(), host);
    Ok(())
}

async fn redeem(host: &RemoteHost, code: &str, name: &str) -> Result<String, String> {
    let tls = dial(host).await.map_err(|e| e.to_string())?;
    let body = serde_json::json!({ "code": code, "name": name }).to_string();
    let request = http::Request::builder()
        .method(http::Method::POST)
        .uri("/v0/pair")
        .header(http::header::CONTENT_TYPE, "application/json")
        .body(body.into_bytes())
        .map_err(|e| e.to_string())?;
    let resp = proxy(tls, request)
        .await
        .map_err(|e| format!("pairing with {} failed: {e}", host.endpoint))?;
    minted_token(&resp)
}

/// `data.token` from the daemon's envelope, or its `error` when it refused.
fn minted_token(resp: &http::Response<Vec<u8>>) -> Result<String, String> {
    let body: serde_json::Value = serde_json::from_slice(resp.body()).unwrap_or_default();
    if !resp.status().is_success() {
        let reason = body["error"].as_str().unwrap_or("no reason given");
        return Err(format!(
            "the host refused pairing ({}): {reason}",
            resp.status()
        ));
    }
    body["data"]["token"]
        .as_str()
        .filter(|token| !token.trim().is_empty())
        .map(str::to_string)
        .ok_or_else(|| "the host's pairing answer carried no token".to_string())
}

/// The keychain write can block on an OS prompt, so it runs on the blocking pool.
async fn keep_token(
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
    token: String,
) -> Result<(), String> {
    let tokens = Arc::clone(tokens);
    let host_id = host_id.to_string();
    tokio::task::spawn_blocking(move || tokens.set(&host_id, &token))
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
    use crate::hosts::{HostRegistry, RemoteHost};
    use crate::tls_pin::fingerprint_of;

    const PAIRED: &str = r#"{"success":true,"data":{"token":"minted-token","clientId":"c1"}}"#;

    fn host(addr: SocketAddr, fingerprint: String) -> RemoteHost {
        RemoteHost {
            endpoint: addr.to_string(),
            fingerprint,
        }
    }

    #[tokio::test]
    async fn pairing_redeems_the_code_over_the_pinned_channel_and_keeps_the_token() {
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, reported) = serve_once_tls(cert, key, Some(response("200 OK", PAIRED))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        pair(
            &registry,
            &keychain,
            HOST_ID,
            host(addr, fingerprint.clone()),
            "123456",
            "laptop",
        )
        .await
        .expect("pairing succeeds");

        assert_eq!(
            keychain.get(HOST_ID).unwrap().as_deref(),
            Some("minted-token")
        );
        let registered = registry.get(HOST_ID).expect("a paired host is registered");
        assert_eq!(registered.endpoint, addr.to_string());
        assert_eq!(registered.fingerprint, fingerprint);
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

    #[tokio::test]
    async fn pairing_with_a_host_presenting_another_certificate_sends_and_keeps_nothing() {
        let (pinned, _) = identity();
        let (impostor_cert, impostor_key) = identity();
        let reply = Some(response("200 OK", PAIRED));
        let (addr, reported) = serve_once_tls(impostor_cert, impostor_key, reply).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let err = pair(
            &registry,
            &keychain,
            HOST_ID,
            host(addr, fingerprint_of(&pinned)),
            "123456",
            "laptop",
        )
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
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let refused = r#"{"success":false,"error":"pairing code is not valid"}"#;
        let reply = Some(response("401 Unauthorized", refused));
        let (addr, _reported) = serve_once_tls(cert, key, reply).await;
        let registry = registry("10.0.0.1:3737".into(), "SHA256:previous".into());
        let keychain = Keychain::holding(&[(HOST_ID, "previous-token")]);

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
            Some("previous-token")
        );
        assert_eq!(registry.get(HOST_ID).unwrap().endpoint, "10.0.0.1:3737");
    }

    #[tokio::test]
    async fn a_token_the_keychain_refuses_leaves_the_host_unregistered() {
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
    async fn pairing_with_a_host_that_never_answers_fails_at_the_deadline() {
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
