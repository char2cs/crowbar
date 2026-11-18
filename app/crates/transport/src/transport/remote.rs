mod dial;
mod host;
mod paired_host;
pub(crate) mod secrets;
mod tls;

pub(crate) use dial::{SERVER_NAME, dial};
pub(crate) use host::Host;
pub(crate) use secrets::Credential;
#[cfg(feature = "os-keychain")]
pub use secrets::OsKeychain;
pub use secrets::{CachedTokenStore, Keychain, MemoryTokenStore, ReadError, TokenStore};
pub use tls::fingerprint_of;
pub(crate) use tls::{
    CertificateDigest, DIGEST_BYTES, Pin, UnpinnedProbeVerifier, certificate_digest,
    client_config_with,
};

use std::sync::Arc;

use http::{HeaderValue, Request, Response, StatusCode, header};
use protocol::HostId;

use crate::error::Error;
use crate::hosts::HostRegistry;
use crate::response::{Forwarded, error_response, marked_response, proxy_answer};
use crate::transport::http_conn::exchange;
use crate::transport::remote::paired_host::PairedHost;
use crate::transport::remote::secrets::credential_io::load;

pub(crate) const REMOTE_BODY_LIMIT: usize = 256 * 1024 * 1024;

pub(crate) fn bearer(token: &str) -> Result<HeaderValue, Error> {
    let mut value = HeaderValue::from_str(&format!("Bearer {token}")).map_err(Error::BadToken)?;
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
        host: Host::paired(endpoint, &credential),
    })
}

fn credential_response(error: &Error) -> Response<Vec<u8>> {
    let status = match error {
        Error::UnknownHost(_) => StatusCode::BAD_GATEWAY,
        Error::NoCredential => StatusCode::UNAUTHORIZED,
        _ => StatusCode::INTERNAL_SERVER_ERROR,
    };
    answer_for(status, error)
}

fn dial_response(error: &Error) -> Response<Vec<u8>> {
    let status = match error {
        Error::HostUnreachable(_) => StatusCode::SERVICE_UNAVAILABLE,
        _ => StatusCode::BAD_GATEWAY,
    };
    answer_for(status, error)
}

fn answer_for(
    status: StatusCode,
    error: &Error,
) -> Response<Vec<u8>> {
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
) -> Forwarded {
    let paired = match paired_host(registry, tokens, host_id).await {
        Ok(paired) => paired,
        Err(e) => return proxy_answer(credential_response(&e)),
    };
    request
        .headers_mut()
        .insert(header::AUTHORIZATION, paired.authorization);
    let tls = match dial(&paired.host).await {
        Ok(tls) => tls,
        Err(e) => return proxy_answer(dial_response(&e)),
    };
    match exchange(Box::new(tls), request, REMOTE_BODY_LIMIT).await {
        Ok(response) => Ok(response),
        Err(Error::TooLarge) => proxy_answer(error_response(
            StatusCode::BAD_GATEWAY,
            "remote response too large",
        )),
        Err(e) => {
            tracing::debug!("remote exchange failed: {e}");
            proxy_answer(error_response(
                StatusCode::BAD_GATEWAY,
                "crowbar proxy error",
            ))
        },
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::response::{
        HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, NO_CREDENTIAL, UNKNOWN_HOST, marker_of,
    };
    use crate::transport::remote::secrets::MemoryTokenStore;

    #[test]
    fn a_bearer_header_is_sensitive_and_refuses_newlines() {
        assert!(bearer("abc").unwrap().is_sensitive());
        assert!(matches!(bearer("a\nb"), Err(Error::BadToken(_))));
    }

    #[tokio::test]
    async fn a_host_with_no_registration_or_credential_is_never_dialled() {
        let registry = HostRegistry::in_memory();
        let tokens: Arc<dyn TokenStore> = Arc::new(MemoryTokenStore::default());
        let id = HostId::new("host-1").unwrap();
        assert!(matches!(
            paired_host(&registry, &tokens, &id).await,
            Err(Error::UnknownHost(_))
        ));
        registry
            .insert_blocking(
                id.clone(),
                crate::hosts::Endpoint::parse("10.0.0.2:7000").unwrap(),
            )
            .unwrap();
        assert!(matches!(
            paired_host(&registry, &tokens, &id).await,
            Err(Error::NoCredential)
        ));
    }

    type Respond = fn(&Error) -> Response<Vec<u8>>;

    type Case = (Respond, Error, u16, Option<&'static str>);

    #[test]
    fn each_failure_is_answered_with_its_status_and_a_marker_the_page_can_act_on() {
        let cases: [Case; 4] = [
            (
                dial_response,
                Error::HostUnreachable("down".into()),
                503,
                Some(HOST_UNREACHABLE),
            ),
            (
                dial_response,
                Error::IdentityChanged("changed".into()),
                502,
                Some(HOST_IDENTITY_CHANGED),
            ),
            (
                credential_response,
                Error::NoCredential,
                401,
                Some(NO_CREDENTIAL),
            ),
            (
                credential_response,
                Error::UnknownHost(HostId::new("host-1").unwrap()),
                502,
                Some(UNKNOWN_HOST),
            ),
        ];
        for (respond, error, status, marker) in cases {
            let answer = respond(&error);
            assert_eq!(answer.status(), status, "{error}");
            assert_eq!(marker_of(&answer), marker, "{error}");
        }
    }
}
