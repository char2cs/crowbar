//! Pairing: a one-time `crowbar pair` code is redeemed over the pinned channel for a
//! bearer token, which goes straight into the keychain and never reaches the webview.

use std::sync::Arc;
use std::time::Duration;

use tauri::{http, State};
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;

use super::remote::{bearer, dial, RemoteHost, REMOTE_BODY_LIMIT};
use super::{open_channel, send, Channel};
use crate::host_tokens::{Credential, TokenStore};
use crate::hosts::endpoint::parse_endpoint;
use crate::hosts::{validate_host_id, HostRegistry};
use crate::tls_pin::{certificate_digest, CertificateDigest, Pin};

/// Bound on the whole redeem: dial, request and answer.
const PAIR_TIMEOUT: Duration = Duration::from_secs(30);

/// Redeems `code` at `endpoint`, trusting only `fingerprint`'s certificate; any failure
/// leaves registry and keychain untouched. Resolves to the daemon's id for this client,
/// which a server-side revoke (`DELETE /v0/clients/:id`) names.
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
    let mut redeemed = within_deadline(&host, redeem(&host, code, name)).await?;
    if let Err(err) = keep_credential(tokens, host_id, &redeemed.credential).await {
        return Err(undo_unkept(&host, &mut redeemed, err).await);
    }
    registry.insert(host_id.to_string(), host.endpoint)?;
    Ok(redeemed.client_id)
}

async fn within_deadline<T>(
    host: &RemoteHost,
    step: impl std::future::Future<Output = Result<T, String>>,
) -> Result<T, String> {
    tokio::time::timeout(PAIR_TIMEOUT, step)
        .await
        .map_err(|_| {
            format!(
                "{} did not finish pairing within {PAIR_TIMEOUT:?}",
                host.endpoint
            )
        })?
}

struct Redeemed {
    credential: Credential,
    client_id: String,
    /// The connection that minted the token, kept for revoking it.
    channel: Channel,
}

/// A token nothing kept could never be used or revoked from here again, so it
/// is revoked now on the connection that minted it. The error says which way that went.
async fn undo_unkept(host: &RemoteHost, redeemed: &mut Redeemed, unkept: String) -> String {
    let what = format!(
        "the keychain could not keep the token for {} ({unkept})",
        host.endpoint
    );
    match within_deadline(host, revoke(redeemed)).await {
        Ok(()) => format!("{what}; the host revoked it"),
        Err(why) => format!("{what}; revoking it on the host failed: {why}"),
    }
}

async fn revoke(redeemed: &mut Redeemed) -> Result<(), String> {
    let request = http::Request::builder()
        .method(http::Method::DELETE)
        .uri(format!("/v0/clients/{}", redeemed.client_id))
        .header(
            http::header::AUTHORIZATION,
            bearer(&redeemed.credential.token)?,
        )
        .body(Vec::new())
        .map_err(|e| e.to_string())?;
    let resp = send(&mut redeemed.channel, request, REMOTE_BODY_LIMIT)
        .await
        .map_err(|e| e.to_string())?;
    if resp.status().is_success() {
        return Ok(());
    }
    Err(format!("the host answered {}", resp.status()))
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
    let failed = |e| format!("pairing with {} failed: {e}", host.endpoint);
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
mod tests;
