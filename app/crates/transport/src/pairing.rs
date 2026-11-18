mod pair_request;
mod probe;
mod redeemed;
mod unrecorded;

pub use pair_request::PairRequest;
pub use probe::probe_fingerprint;

use std::future::Future;
use std::sync::Arc;
use std::time::Duration;

use http::{Method, Request, Response, header};
use protocol::HostId;
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;

use crate::error::Error;
use crate::hosts::Endpoint;
use crate::hosts::HostRegistry;
use crate::hosts::check_registrable;
use crate::pairing::redeemed::Redeemed;
use crate::pairing::unrecorded::Unrecorded;
use crate::transport::Credential;
use crate::transport::Host;
use crate::transport::Pin;
use crate::transport::TokenStore;
use crate::transport::dial;
use crate::transport::remote::secrets::credential_io;
use crate::transport::{CertificateDigest, certificate_digest};
use crate::transport::{REMOTE_BODY_LIMIT, bearer};
use crate::transport::{open_channel, send};

const PAIR_TIMEOUT: Duration = Duration::from_secs(30);

/// Redeems the one-time code at the host, trusting only the confirmed fingerprint's certificate,
/// keeps the token in `tokens` and registers the host. On any failure nothing is left behind: the
/// registry is restored, no credential is stored, and the token is revoked on the host. Resolves
/// to the daemon's id for this client, which a server-side revoke (`DELETE /v0/clients/:id`)
/// names.
///
/// # Errors
/// Any pairing failure: a bad id or address, an unreachable or impostor host, a refused code, or
/// a host list or keychain that cannot record the pairing ([`Error::NotRecorded`]).
pub async fn pair_remote_host(
    registry: &Arc<HostRegistry>,
    tokens: &Arc<dyn TokenStore>,
    request: &PairRequest<'_>,
) -> Result<String, Error> {
    let host_id = HostId::new(request.host_id)?;
    check_registrable(&host_id)?;
    let host = Host {
        endpoint: Endpoint::parse(request.endpoint)?,
        pin: Pin::Confirmed(request.fingerprint.to_owned()),
    };
    let mut redeemed = within_deadline(&host.endpoint, redeem(&host, request)).await?;
    match record(registry, tokens, &host_id, &redeemed).await {
        Ok(()) => Ok(redeemed.client_id),
        Err(unrecorded) => Err(undo(&mut redeemed, unrecorded).await),
    }
}

async fn within_deadline<T>(
    endpoint: &Endpoint,
    step: impl Future<Output = Result<T, Error>>,
) -> Result<T, Error> {
    let paired = tokio::time::timeout(PAIR_TIMEOUT, step).await;
    paired.map_err(|deadline| Error::PairTimeout {
        endpoint: endpoint.to_string(),
        deadline,
    })?
}

async fn record(
    registry: &Arc<HostRegistry>,
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
    redeemed: &Redeemed,
) -> Result<(), Unrecorded> {
    let previous = registry.get(host_id);
    registry
        .insert(host_id.clone(), redeemed.endpoint.clone())
        .await
        .map_err(|e| Unrecorded {
            stage: "host list",
            cause: e.to_string(),
            rollback: "the host list is unchanged".to_owned(),
        })?;
    let Err(e) = credential_io::persist_credential(tokens, host_id, &redeemed.credential).await
    else {
        return Ok(());
    };
    Err(Unrecorded {
        stage: "keychain",
        cause: e.to_string(),
        rollback: restore(registry, host_id, previous).await,
    })
}

async fn restore(
    registry: &Arc<HostRegistry>,
    host_id: &HostId,
    previous: Option<Endpoint>,
) -> String {
    let restored = match previous {
        Some(endpoint) => registry.insert(host_id.clone(), endpoint).await,
        None => registry.remove(host_id.clone()).await,
    };
    match restored {
        Ok(()) => "the host list is restored".to_owned(),
        Err(e) => format!("restoring the host list failed: {e}"),
    }
}

async fn undo(
    redeemed: &mut Redeemed,
    unrecorded: Unrecorded,
) -> Error {
    let endpoint = redeemed.endpoint.clone();
    let revoked = within_deadline(&endpoint, revoke(redeemed)).await;
    let revoke_note = match revoked {
        Ok(()) => "the host revoked the token".to_owned(),
        Err(why) => format!("revoking the token on the host failed: {why}"),
    };
    Error::NotRecorded {
        endpoint: endpoint.to_string(),
        stage: unrecorded.stage,
        cause: unrecorded.cause,
        cleanup: format!("{}; {revoke_note}", unrecorded.rollback),
    }
}

async fn revoke(redeemed: &mut Redeemed) -> Result<(), Error> {
    let exchange_failed = |source| Error::PairExchange {
        endpoint: redeemed.endpoint.to_string(),
        source: Box::new(source),
    };
    let request = Request::builder()
        .method(Method::DELETE)
        .uri(format!("/v0/clients/{}", redeemed.client_id))
        .header(header::AUTHORIZATION, bearer(redeemed.credential.token())?)
        .body(Vec::new())
        .map_err(|e| exchange_failed(Error::bad_request(e)))?;
    let response = send(&mut redeemed.channel, request, REMOTE_BODY_LIMIT)
        .await
        .map_err(exchange_failed)?;
    if response.status().is_success() {
        return Ok(());
    }
    Err(Error::Refused {
        status: response.status().as_u16(),
        reason: "revoke declined".to_owned(),
    })
}

async fn redeem(
    host: &Host,
    request: &PairRequest<'_>,
) -> Result<Redeemed, Error> {
    let exchange_failed = |source| Error::PairExchange {
        endpoint: host.endpoint.to_string(),
        source: Box::new(source),
    };
    let tls = dial(host).await?;
    let pin = peer_digest(&tls).ok_or_else(|| Error::NoCertificate(host.endpoint.to_string()))?;
    let body = serde_json::json!({ "code": request.code, "name": request.name }).to_string();
    let http_request = Request::builder()
        .method(Method::POST)
        .uri("/v0/pair")
        .header(header::CONTENT_TYPE, "application/json")
        .body(body.into_bytes())
        .map_err(|e| exchange_failed(Error::bad_request(e)))?;
    let mut channel = open_channel(Box::new(tls)).await.map_err(exchange_failed)?;
    let response = send(&mut channel, http_request, REMOTE_BODY_LIMIT)
        .await
        .map_err(exchange_failed)?;
    let (token, client_id) = pairing_answer(&response)?;
    Ok(Redeemed {
        endpoint: host.endpoint.clone(),
        credential: Credential::new(token, pin),
        client_id,
        channel,
    })
}

fn peer_digest(tls: &TlsStream<TcpStream>) -> Option<CertificateDigest> {
    let chain = tls.get_ref().1.peer_certificates()?;
    chain.first().map(|cert| certificate_digest(cert))
}

fn refusal(
    status: http::StatusCode,
    reason: &str,
) -> Error {
    Error::Refused {
        status: status.as_u16(),
        reason: reason.to_owned(),
    }
}

fn pairing_answer(response: &Response<Vec<u8>>) -> Result<(String, String), Error> {
    let body: serde_json::Value = serde_json::from_slice(response.body()).unwrap_or_default();
    if !response.status().is_success() {
        let reason = body
            .get("error")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("no reason given");
        return Err(refusal(response.status(), reason));
    }
    let field = |name: &'static str| {
        body.get("data")
            .and_then(|data| data.get(name))
            .and_then(serde_json::Value::as_str)
            .filter(|value| !value.trim().is_empty())
            .map(str::to_owned)
            .ok_or(Error::MissingField(name))
    };
    Ok((field("token")?, field("clientId")?))
}

#[cfg(test)]
mod tests {
    use super::*;

    fn answer(
        status: u16,
        body: &str,
    ) -> Response<Vec<u8>> {
        let mut response = Response::new(body.as_bytes().to_vec());
        *response.status_mut() = http::StatusCode::from_u16(status).unwrap();
        response
    }

    fn verdict(response: &Response<Vec<u8>>) -> String {
        match pairing_answer(response) {
            Ok((token, client)) => format!("paired {token}/{client}"),
            Err(Error::Refused { status, reason }) => format!("refused {status}: {reason}"),
            Err(Error::MissingField(name)) => format!("missing {name}"),
            Err(other) => panic!("unexpected {other}"),
        }
    }

    /// (status, body, verdict)
    const CASES: [(u16, &str, &str); 6] = [
        (
            200,
            r#"{"data":{"token":"t","clientId":"c1"}}"#,
            "paired t/c1",
        ),
        (
            403,
            r#"{"error":"code expired"}"#,
            "refused 403: code expired",
        ),
        (200, r#"{"data":{"clientId":"c"}}"#, "missing token"),
        (
            200,
            r#"{"data":{"token":" ","clientId":"c"}}"#,
            "missing token",
        ),
        (200, r#"{"data":{"token":"t"}}"#, "missing clientId"),
        (200, "nonsense", "missing token"),
    ];

    #[test]
    fn a_pairing_answer_yields_its_credentials_or_names_what_went_wrong() {
        for (status, body, expected) in CASES {
            assert_eq!(verdict(&answer(status, body)), expected, "{body}");
        }
    }
}
