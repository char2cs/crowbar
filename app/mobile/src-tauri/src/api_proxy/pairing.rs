use std::future::Future;
use std::sync::Arc;
use std::time::Duration;

use tauri::{http, State};
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;

use super::channel::{open_channel, send};
use super::remote::{bearer, dial, RemoteHost, REMOTE_BODY_LIMIT};
use crate::host_error::HostError;
use crate::host_tokens::{Credential, TokenStore};
use crate::hosts::endpoint::parse_endpoint;
use crate::hosts::{validate_host_id, HostRegistry};
use crate::tls_pin::{certificate_digest, CertificateDigest, Pin};
use pairing_request::PairingRequest;
use redeemed::Redeemed;

mod pairing_request;
mod redeemed;

const PAIR_TIMEOUT: Duration = Duration::from_secs(30);

#[tauri::command]
pub(crate) async fn pair_remote_host(
    request: PairingRequest,
    registry: State<'_, HostRegistry>,
    tokens: State<'_, Arc<dyn TokenStore>>,
) -> Result<String, HostError> {
    pair(&registry, &tokens, request).await
}

async fn pair(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    request: PairingRequest,
) -> Result<String, HostError> {
    let host = RemoteHost {
        endpoint: parse_endpoint(&request.endpoint)?,
        pin: Pin::Confirmed(request.fingerprint),
    };
    validate_host_id(&request.host_id)?;
    let redeeming = redeem(&host, &request.code, &request.name);
    let mut redeemed = within_deadline(&host, redeeming).await?;
    if let Err(err) = keep_credential(tokens, &request.host_id, &redeemed.credential).await {
        return Err(undo_unkept(&host, &mut redeemed, err).await);
    }
    registry.insert(request.host_id, host.endpoint)?;
    Ok(redeemed.client_id)
}

async fn within_deadline<T>(
    host: &RemoteHost,
    step: impl Future<Output = Result<T, HostError>>,
) -> Result<T, HostError> {
    tokio::time::timeout(PAIR_TIMEOUT, step)
        .await
        .map_err(|source| HostError::PairingTimeout {
            endpoint: host.endpoint.to_string(),
            limit: PAIR_TIMEOUT,
            source,
        })?
}

async fn undo_unkept(
    host: &RemoteHost,
    redeemed: &mut Redeemed,
    unkept: HostError,
) -> HostError {
    let revocation = within_deadline(host, revoke(redeemed)).await.err();
    HostError::KeychainRefused {
        endpoint: host.endpoint.to_string(),
        source: Box::new(unkept),
        revocation: revocation.map(Box::new),
    }
}

async fn revoke(redeemed: &mut Redeemed) -> Result<(), HostError> {
    let request = http::Request::builder()
        .method(http::Method::DELETE)
        .uri(format!("/v0/clients/{}", redeemed.client_id))
        .header(
            http::header::AUTHORIZATION,
            bearer(&redeemed.credential.token)?,
        )
        .body(Vec::new())
        .map_err(HostError::BadRequest)?;
    let resp = send(&mut redeemed.channel, request, REMOTE_BODY_LIMIT)
        .await
        .map_err(HostError::Transport)?;
    if resp.status().is_success() {
        return Ok(());
    }
    Err(HostError::RevokeRefused(resp.status()))
}

fn redeem_request(
    code: &str,
    name: &str,
) -> Result<http::Request<Vec<u8>>, HostError> {
    let body = serde_json::json!({ "code": code, "name": name }).to_string();
    http::Request::builder()
        .method(http::Method::POST)
        .uri("/v0/pair")
        .header(http::header::CONTENT_TYPE, "application/json")
        .body(body.into_bytes())
        .map_err(HostError::BadRequest)
}

async fn redeem(
    host: &RemoteHost,
    code: &str,
    name: &str,
) -> Result<Redeemed, HostError> {
    let endpoint = host.endpoint.to_string();
    let tls = dial(host).await.map_err(|e| HostError::Dial(Box::new(e)))?;
    let pin = peer_digest(&tls).ok_or_else(|| HostError::NoCertificate(endpoint.clone()))?;
    let request = redeem_request(code, name)?;
    let failed = |source| HostError::PairingTransport {
        endpoint: endpoint.clone(),
        source,
    };
    let mut channel = open_channel(tls).await.map_err(failed)?;
    let resp = send(&mut channel, request, REMOTE_BODY_LIMIT)
        .await
        .map_err(failed)?;
    let (token, client_id) = pairing_answer(&resp)?;
    Ok(Redeemed {
        credential: Credential { token, pin },
        client_id,
        channel,
    })
}

fn peer_digest(tls: &TlsStream<TcpStream>) -> Option<CertificateDigest> {
    let chain = tls.get_ref().1.peer_certificates()?;
    chain.first().map(|cert| certificate_digest(cert))
}

fn pairing_answer(resp: &http::Response<Vec<u8>>) -> Result<(String, String), HostError> {
    let body: serde_json::Value = serde_json::from_slice(resp.body()).unwrap_or_default();
    if !resp.status().is_success() {
        let reason = body
            .get("error")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("no reason given");
        return Err(HostError::pairing_refused(resp.status(), reason));
    }
    let field = |name: &str| {
        body.get("data")
            .and_then(|data| data.get(name))
            .and_then(serde_json::Value::as_str)
            .filter(|value| !value.trim().is_empty())
            .map(str::to_owned)
            .ok_or_else(|| HostError::PairingAnswerLacks(name.to_owned()))
    };
    Ok((field("token")?, field("clientId")?))
}

async fn keep_credential(
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
    credential: &Credential,
) -> Result<(), HostError> {
    let tokens = Arc::clone(tokens);
    let host_id = host_id.to_owned();
    let entry = credential.encode();
    tokio::task::spawn_blocking(move || tokens.set(&host_id, &entry))
        .await
        .map_err(HostError::Interrupted)?
        .map_err(HostError::Token)
}

#[cfg(test)]
mod tests {
    use std::net::SocketAddr;

    use rustls::pki_types::{CertificateDer, PrivateKeyDer};
    use serde_json::{json, Value};

    use super::*;
    use crate::test_support::proxy_fakes::{
        header, identity, kept_alive, registry, response, serve_once_tls, serve_tls_exchanges,
        Keychain, HOST_ID,
    };
    use crate::tls_pin::fingerprint_of;

    const PAIRED: &str = r#"{"success":true,"data":{"token":"minted-token","clientId":"c1"}}"#;

    fn request(
        host_id: &str,
        endpoint: String,
        cert: &CertificateDer<'_>,
    ) -> PairingRequest {
        PairingRequest {
            host_id: host_id.to_owned(),
            endpoint,
            fingerprint: fingerprint_of(cert),
            code: "123456".to_owned(),
            name: "laptop".to_owned(),
        }
    }

    fn registered(registry: &HostRegistry) -> Option<String> {
        registry.get(HOST_ID).map(|endpoint| endpoint.to_string())
    }

    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn serve(
        replies: Option<Vec<u8>>
    ) -> (
        CertificateDer<'static>,
        SocketAddr,
        tokio::sync::oneshot::Receiver<Result<String, String>>,
    ) {
        let (cert, key): (_, PrivateKeyDer<'static>) = identity();
        let (addr, reported) = serve_once_tls(cert.clone(), key, replies).await;
        (cert, addr, reported)
    }

    #[tokio::test]
    async fn pairing_redeems_the_code_over_the_pinned_channel_and_keeps_the_token() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, addr, reported) = serve(Some(response("200 OK", PAIRED))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let request = request(HOST_ID, addr.to_string(), &cert);
        let client_id = pair(&registry, &keychain, request)
            .await
            .expect("pairing succeeds");

        assert_eq!(client_id, "c1", "the id a server-side revoke names");
        let stored = keychain
            .get(HOST_ID)
            .unwrap()
            .expect("a credential is kept");
        let expected = Credential {
            token: "minted-token".into(),
            pin: certificate_digest(&cert),
        };
        assert_eq!(
            Credential::decode(&stored),
            Some(expected),
            "bound to the whole digest"
        );
        assert_eq!(registered(&registry), Some(addr.to_string()));
        let sent = reported
            .await
            .unwrap()
            .expect("handshake with the pinned host");
        assert!(sent.starts_with("POST /v0/pair HTTP/1.1\r\n"), "{sent}");
        assert_eq!(header(&sent, "content-type"), Some("application/json"));
        assert_eq!(header(&sent, "authorization"), None);
        let body: Value = serde_json::from_str(sent.split_once("\r\n\r\n").unwrap().1).unwrap();
        assert_eq!(body, json!({"code": "123456", "name": "laptop"}));
    }

    #[tokio::test]
    async fn a_host_paired_by_name_is_dialed_and_registered_by_that_name() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, addr, reported) = serve(Some(response("200 OK", PAIRED))).await;
        let endpoint = format!("localhost:{}", addr.port());
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let request = request(HOST_ID, endpoint.clone(), &cert);
        pair(&registry, &keychain, request)
            .await
            .expect("pairing by name succeeds");

        assert_eq!(registered(&registry), Some(endpoint));
        let sent = reported.await.unwrap().expect("handshake by name");
        assert!(sent.starts_with("POST /v0/pair "), "{sent}");
    }

    #[tokio::test]
    async fn pairing_with_a_host_presenting_another_certificate_sends_and_keeps_nothing() {
        let _serialised = crate::test_support::fd_tests().await;
        let (pinned, _) = identity();
        let (_impostor, addr, reported) = serve(Some(response("200 OK", PAIRED))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let request = request(HOST_ID, format!("localhost:{}", addr.port()), &pinned);
        let err = pair(&registry, &keychain, request)
            .await
            .expect_err("an impostor");

        assert!(err.to_string().contains("identity changed"), "{err}");
        let served = reported.await.unwrap();
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
        let refused = r#"{"success":false,"error":"pairing code is not valid"}"#;
        let (cert, addr, _reported) = serve(Some(response("401 Unauthorized", refused))).await;
        let registry = registry("10.0.0.1:3737");
        let keychain = Keychain::holding(&[(HOST_ID, "previous-credential")]);

        let request = request(HOST_ID, addr.to_string(), &cert);
        let err = pair(&registry, &keychain, request)
            .await
            .expect_err("a refused code");

        assert!(
            err.to_string().contains("pairing code is not valid"),
            "{err}"
        );
        let kept = keychain.get(HOST_ID).unwrap();
        assert_eq!(kept.as_deref(), Some("previous-credential"));
        assert_eq!(registered(&registry).as_deref(), Some("10.0.0.1:3737"));
    }

    #[tokio::test]
    async fn an_answer_without_a_client_id_pairs_nothing() {
        let _serialised = crate::test_support::fd_tests().await;
        let tokenonly = r#"{"success":true,"data":{"token":"minted-token"}}"#;
        let (cert, addr, _reported) = serve(Some(response("200 OK", tokenonly))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let request = request(HOST_ID, addr.to_string(), &cert);
        let err = pair(&registry, &keychain, request)
            .await
            .expect_err("nothing to revoke");

        assert!(err.to_string().contains("no clientId"), "{err}");
        assert_eq!(keychain.get(HOST_ID).unwrap(), None);
        assert!(registry.get(HOST_ID).is_none());
    }

    #[tokio::test]
    #[ignore = "real-infra: pins certificates against a loopback TLS server; the rustls verifier is the unit under test"]
    async fn a_token_the_keychain_refuses_is_revoked_and_the_host_left_unregistered() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let revoked = r#"{"success":true,"data":{"revoked":true}}"#;
        let replies = vec![kept_alive("200 OK", PAIRED), kept_alive("200 OK", revoked)];
        let (addr, mut reported) = serve_tls_exchanges(cert.clone(), key, replies).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::broken("keychain is locked");

        let request = request(HOST_ID, addr.to_string(), &cert);
        let err = pair(&registry, &keychain, request)
            .await
            .expect_err("unstorable");

        assert!(err.to_string().contains("keychain is locked"), "{err}");
        assert!(err.to_string().ends_with("the host revoked it"), "{err}");
        assert!(registry.get(HOST_ID).is_none());
        let redeem = reported.recv().await.expect("the code was redeemed");
        assert!(redeem.starts_with("POST /v0/pair "), "{redeem}");
        let revoke = reported.recv().await.expect("the client was revoked");
        assert!(
            revoke.starts_with("DELETE /v0/clients/c1 HTTP/1.1\r\n"),
            "{revoke}"
        );
        assert_eq!(
            header(&revoke, "authorization"),
            Some("Bearer minted-token")
        );
    }

    #[tokio::test]
    async fn a_refused_token_the_host_will_not_revoke_says_so() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, addr, _reported) = serve(Some(response("200 OK", PAIRED))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::broken("keychain is locked");

        let request = request(HOST_ID, addr.to_string(), &cert);
        let err = pair(&registry, &keychain, request)
            .await
            .expect_err("unstorable");

        assert!(err.to_string().contains("keychain is locked"), "{err}");
        assert!(
            err.to_string().contains("revoking it on the host failed"),
            "{err}"
        );
        assert!(registry.get(HOST_ID).is_none());
    }

    #[tokio::test]
    async fn a_host_id_the_router_would_not_send_to_that_host_spends_no_code() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, addr, mut reported) = serve(Some(response("200 OK", PAIRED))).await;
        let registry = HostRegistry::default();
        let keychain = Keychain::holding(&[]);

        let request = request("localhost", addr.to_string(), &cert);
        let err = pair(&registry, &keychain, request)
            .await
            .expect_err("the local daemon's name");

        assert!(err.to_string().contains("host id"), "{err}");
        assert!(reported.try_recv().is_err(), "the host was dialed");
        assert_eq!(keychain.get("localhost").unwrap(), None);
    }

    #[tokio::test]
    async fn pairing_with_a_host_that_never_answers_fails_at_the_deadline() {
        pairing_with_a_host_that_never_answers_f_run().await;
    }

    async fn pairing_with_a_host_that_never_answers_f_run() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, addr, reported) = serve(None).await;
        let request = request(HOST_ID, addr.to_string(), &cert);
        let pairing = tokio::spawn(async move {
            let registry = HostRegistry::default();
            let keychain = Keychain::holding(&[]);
            let paired = pair(&registry, &keychain, request).await;
            (paired, registry.get(HOST_ID), keychain.get(HOST_ID))
        });

        reported
            .await
            .unwrap()
            .expect("handshake with the pinned host");
        tokio::time::pause();
        let timed = tokio::time::timeout(Duration::from_secs(600), pairing);
        let (paired, registered, kept) = timed.await.expect("must fail at the deadline").unwrap();

        let err = paired.expect_err("a silent host is not paired");
        assert!(err.to_string().contains("did not finish pairing"), "{err}");
        assert!(matches!(kept, Ok(None)));
        assert!(registered.is_none());
    }

    type Case = (
        u16,
        &'static str,
        Result<(&'static str, &'static str), &'static str>,
    );

    #[test]
    fn the_daemons_pairing_answer_is_a_token_and_client_id_or_a_reason() {
        const CASES: &[Case] = &[
            (
                200,
                r#"{"data":{"token":"t","clientId":"c"}}"#,
                Ok(("t", "c")),
            ),
            (
                200,
                r#"{"data":{"token":" ","clientId":"c"}}"#,
                Err("carried no token"),
            ),
            (200, r#"{"data":{"token":"t"}}"#, Err("carried no clientId")),
            (200, "not json", Err("carried no token")),
            (
                401,
                r#"{"error":"bad code"}"#,
                Err("refused pairing (401 Unauthorized): bad code"),
            ),
            (
                500,
                "",
                Err("refused pairing (500 Internal Server Error): no reason given"),
            ),
        ];
        for (status, body, expected) in CASES {
            let resp = http::Response::builder()
                .status(*status)
                .body(body.as_bytes().to_vec())
                .unwrap();

            let answer = pairing_answer(&resp);

            match (answer, expected) {
                (Ok(got), Ok((token, id))) => assert_eq!(got, (token.to_string(), id.to_string())),
                (Err(got), Err(fragment)) => assert!(got.to_string().contains(fragment), "{got}"),
                (got, _) => panic!("{body}: unexpected {got:?}"),
            }
        }
    }
}
