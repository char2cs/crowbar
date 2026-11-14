use std::future::Future;
use std::sync::Arc;
use std::time::Duration;

use http::{Method, Request, Response, header};
use protocol::HostId;
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;

use crate::credential::Credential;
use crate::dial::dial;
use crate::endpoint::Endpoint;
use crate::error::Error;
use crate::fingerprint::{CertificateDigest, certificate_digest};
use crate::host_registry::HostRegistry;
use crate::host_target::check_registrable;
use crate::http_conn::{open_channel, send};
use crate::pair_request::PairRequest;
use crate::pin::Pin;
use crate::redeemed::Redeemed;
use crate::remote::{REMOTE_BODY_LIMIT, bearer};
use crate::remote_host::RemoteHost;
use crate::token_store::TokenStore;

/// Bound on the whole redeem: dial, request and answer.
pub const PAIR_TIMEOUT: Duration = Duration::from_secs(30);

/// Redeems the one-time code at the host, trusting only the confirmed fingerprint's certificate,
/// keeps the token in `tokens` and registers the host. On any failure the registry and keychain
/// are untouched. Resolves to the daemon's id for this client, which a server-side revoke
/// (`DELETE /v0/clients/:id`) names.
///
/// # Errors
/// Any pairing failure: a bad id or address, an unreachable or impostor host, a refused code, a
/// keychain that cannot keep the token (the token is then revoked on the host).
pub async fn pair_remote_host(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    request: &PairRequest<'_>,
) -> Result<String, Error> {
    let host_id = HostId::new(request.host_id)?;
    check_registrable(&host_id)?;
    let host = RemoteHost {
        endpoint: Endpoint::parse(request.endpoint)?,
        pin: Pin::Confirmed(request.fingerprint.to_owned()),
    };
    let mut redeemed = within_deadline(&host, redeem(&host, request)).await?;
    keep_or_revoke(tokens, &host_id, &mut redeemed).await?;
    registry.insert(host_id, host.endpoint)?;
    Ok(redeemed.client_id)
}

async fn within_deadline<T>(
    host: &RemoteHost,
    step: impl Future<Output = Result<T, Error>>,
) -> Result<T, Error> {
    match tokio::time::timeout(PAIR_TIMEOUT, step).await {
        Ok(result) => result,
        Err(_) => Err(Error::PairTimeout(host.endpoint.clone())),
    }
}

async fn keep_or_revoke(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
    redeemed: &mut Redeemed,
) -> Result<(), Error> {
    let Err(keychain) = keep_credential(tokens, host_id, &redeemed.credential).await else {
        return Ok(());
    };
    let endpoint = redeemed.endpoint.clone();
    let revoked = within_deadline_for(&endpoint, revoke(redeemed)).await;
    Err(Error::Unkept {
        endpoint,
        keychain,
        revoke: match revoked {
            Ok(()) => "the host revoked it".to_owned(),
            Err(why) => format!("revoking it on the host failed: {why}"),
        },
    })
}

async fn within_deadline_for(
    endpoint: &Endpoint,
    step: impl Future<Output = Result<(), Error>>,
) -> Result<(), Error> {
    match tokio::time::timeout(PAIR_TIMEOUT, step).await {
        Ok(result) => result,
        Err(_) => Err(Error::PairTimeout(endpoint.clone())),
    }
}

async fn keep_credential(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
    credential: &Credential,
) -> Result<(), String> {
    let store = Arc::clone(tokens);
    let key = host_id.to_string();
    let entry = credential.encode();
    tokio::task::spawn_blocking(move || store.set(&key, &entry))
        .await
        .map_err(|e| e.to_string())?
        .map_err(|e| e.to_string())
}

async fn revoke(redeemed: &mut Redeemed) -> Result<(), Error> {
    let exchange_failed = |source| Error::PairExchange {
        endpoint: redeemed.endpoint.clone(),
        source: Box::new(source),
    };
    let request = Request::builder()
        .method(Method::DELETE)
        .uri(format!("/v0/clients/{}", redeemed.client_id))
        .header(header::AUTHORIZATION, bearer(&redeemed.credential.token)?)
        .body(Vec::new())
        .map_err(|e| exchange_failed(e.into()))?;
    let response = send(&mut redeemed.channel, request, REMOTE_BODY_LIMIT)
        .await
        .map_err(exchange_failed)?;
    if response.status().is_success() {
        return Ok(());
    }
    Err(Error::Refused {
        status: response.status(),
        reason: "revoke declined".to_owned(),
    })
}

async fn redeem(host: &RemoteHost, request: &PairRequest<'_>) -> Result<Redeemed, Error> {
    let exchange_failed = |source| Error::PairExchange {
        endpoint: host.endpoint.clone(),
        source: Box::new(source),
    };
    let tls = dial(host).await?;
    let pin = peer_digest(&tls).ok_or_else(|| Error::NoCertificate(host.endpoint.clone()))?;
    let body = serde_json::json!({ "code": request.code, "name": request.name }).to_string();
    let http_request = Request::builder()
        .method(Method::POST)
        .uri("/v0/pair")
        .header(header::CONTENT_TYPE, "application/json")
        .body(body.into_bytes())
        .map_err(|e| exchange_failed(e.into()))?;
    let mut channel = open_channel(Box::new(tls)).await.map_err(exchange_failed)?;
    let response = send(&mut channel, http_request, REMOTE_BODY_LIMIT)
        .await
        .map_err(exchange_failed)?;
    let (token, client_id) = pairing_answer(&response)?;
    Ok(Redeemed {
        endpoint: host.endpoint.clone(),
        credential: Credential { token, pin },
        client_id,
        channel,
    })
}

fn peer_digest(tls: &TlsStream<TcpStream>) -> Option<CertificateDigest> {
    let chain = tls.get_ref().1.peer_certificates()?;
    chain.first().map(|cert| certificate_digest(cert))
}

fn pairing_answer(response: &Response<Vec<u8>>) -> Result<(String, String), Error> {
    let body: serde_json::Value = serde_json::from_slice(response.body()).unwrap_or_default();
    if !response.status().is_success() {
        let reason = body
            .get("error")
            .and_then(serde_json::Value::as_str)
            .unwrap_or("no reason given");
        return Err(Error::Refused {
            status: response.status(),
            reason: reason.to_owned(),
        });
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
#[path = "pairing_tests.rs"]
mod tests;
