use std::sync::Arc;

use http::{HeaderValue, Request, Response, header};
use protocol::HostId;

use crate::credential::Credential;
use crate::dial::dial;
use crate::error::Error;
use crate::host_registry::HostRegistry;
use crate::http_conn::exchange;
use crate::paired_host::PairedHost;
use crate::remote_host::RemoteHost;
use crate::response::{error_response, marked_response};
use crate::token_store::TokenStore;

/// Most bytes the proxy buffers of one remote host's answer. The webview gets a body whole, so a
/// hostile host could otherwise stream until the desktop runs out of memory; this is far above
/// any honest answer.
pub const REMOTE_BODY_LIMIT: usize = 256 * 1024 * 1024;

/// What pairing stored for `host_id`; any other keychain entry counts as none. The keychain
/// read can block on an OS prompt, so it runs on the blocking pool rather than an async worker.
///
/// # Errors
/// [`Error::NoCredential`] when nothing usable is stored, [`Error::Store`] when the keychain
/// cannot be read.
pub async fn stored_credential(
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
) -> Result<Credential, Error> {
    let store = Arc::clone(tokens);
    let key = host_id.to_string();
    let entry = tokio::task::spawn_blocking(move || store.get(&key))
        .await
        .map_err(|e| Error::CredentialTask(e.to_string()))??;
    entry
        .as_deref()
        .and_then(Credential::decode)
        .ok_or(Error::NoCredential)
}

/// `Bearer <token>`, marked sensitive so it never reaches a debug log.
///
/// # Errors
/// [`Error::BadToken`] when the token cannot be a header value.
pub fn bearer(token: &str) -> Result<HeaderValue, Error> {
    let mut value =
        HeaderValue::from_str(&format!("Bearer {token}")).map_err(|_| Error::BadToken)?;
    value.set_sensitive(true);
    Ok(value)
}

/// Read afresh on every use and before anything is dialled, so a host with no credential never
/// sees a request and an unregistered one is unreachable at once.
///
/// # Errors
/// [`Error::UnknownHost`], or any credential lookup failure.
pub async fn paired_host(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
) -> Result<PairedHost, Error> {
    let endpoint = registry
        .get(host_id)
        .ok_or_else(|| Error::UnknownHost(host_id.clone()))?;
    let credential = stored_credential(tokens, host_id).await?;
    Ok(PairedHost {
        id: host_id.clone(),
        authorization: bearer(&credential.token)?,
        host: RemoteHost::paired(endpoint, &credential),
    })
}

fn credential_response(error: &Error) -> Response<Vec<u8>> {
    let status = match error {
        Error::UnknownHost(_) => 502,
        Error::NoCredential => 401,
        _ => 500,
    };
    error_response(status, &error.to_string())
}

fn dial_response(error: &Error) -> Response<Vec<u8>> {
    let status = if matches!(error, Error::HostUnreachable(_)) {
        503
    } else {
        502
    };
    match error.marker() {
        Some(marker) => marked_response(status, marker, &error.to_string()),
        None => error_response(status, &error.to_string()),
    }
}

/// Relays `request` to paired host `host_id`. The credential is resolved before dialling, so a
/// host with none never receives an unauthenticated request.
pub async fn forward_remote(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
    mut request: Request<Vec<u8>>,
) -> Response<Vec<u8>> {
    let paired = match paired_host(registry, tokens, host_id).await {
        Ok(paired) => paired,
        Err(e) => return credential_response(&e),
    };
    request
        .headers_mut()
        .insert(header::AUTHORIZATION, paired.authorization);
    let tls = match dial(&paired.host).await {
        Ok(tls) => tls,
        Err(e) => return dial_response(&e),
    };
    match exchange(Box::new(tls), request, REMOTE_BODY_LIMIT).await {
        Ok(response) => response,
        Err(Error::TooLarge) => error_response(502, "remote response too large"),
        Err(e) => error_response(502, &format!("crowbar proxy error: {e}")),
    }
}

#[cfg(test)]
#[path = "remote_tests.rs"]
mod tests;
