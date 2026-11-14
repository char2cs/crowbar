use std::sync::Arc;

use http::{HeaderValue, Request, Response, header};
use protocol::HostId;

use crate::credential_io::load;
use crate::dial::dial;
use crate::error::Error;
use crate::host_registry::HostRegistry;
use crate::http_conn::exchange;
use crate::paired_host::PairedHost;
use crate::remote_host::RemoteHost;
use crate::response::{error_response, marked_response};
use crate::token_store::TokenStore;

pub(crate) const REMOTE_BODY_LIMIT: usize = 256 * 1024 * 1024;

pub(crate) fn bearer(token: &str) -> Result<HeaderValue, Error> {
    let mut value =
        HeaderValue::from_str(&format!("Bearer {token}")).map_err(|_| Error::BadToken)?;
    value.set_sensitive(true);
    Ok(value)
}

pub(crate) async fn paired_host(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &HostId,
) -> Result<PairedHost, Error> {
    let endpoint = registry
        .get(host_id)
        .ok_or_else(|| Error::UnknownHost(host_id.clone()))?;
    let credential = load(tokens, host_id).await?;
    Ok(PairedHost {
        authorization: bearer(credential.token())?,
        host: RemoteHost::paired(endpoint, &credential),
    })
}

fn credential_response(error: &Error) -> Response<Vec<u8>> {
    let status = match error {
        Error::UnknownHost(_) => 502,
        Error::NoCredential => 401,
        _ => 500,
    };
    answer_for(status, error)
}

fn dial_response(error: &Error) -> Response<Vec<u8>> {
    let status = if matches!(error, Error::HostUnreachable(_)) {
        503
    } else {
        502
    };
    answer_for(status, error)
}

fn answer_for(status: u16, error: &Error) -> Response<Vec<u8>> {
    match error.marker() {
        Some(marker) => marked_response(status, marker, &error.to_string()),
        None => error_response(status, &error.to_string()),
    }
}

pub(crate) async fn forward_remote(
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
        Err(e) => {
            tracing::debug!("remote exchange failed: {e}");
            error_response(502, "crowbar proxy error")
        },
    }
}

#[cfg(test)]
#[path = "remote_tests.rs"]
mod tests;
