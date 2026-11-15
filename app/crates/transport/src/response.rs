use http::{HeaderValue, Response, StatusCode, header};

use crate::proxy_answer::ProxyAnswer;

/// Marks a response that is the proxy's, not the daemon's; the value says why.
pub const PROXY_HEADER: &str = "x-crowbar-proxy";
/// The local daemon socket: nothing was listening.
pub const DAEMON_UNAVAILABLE: &str = "daemon-unavailable";
/// A remote host did not answer: refused, timed out, or dropped the handshake.
pub const HOST_UNREACHABLE: &str = "host-unreachable";
/// A remote host answered, but not with the identity pinned at pairing.
pub const HOST_IDENTITY_CHANGED: &str = "host-identity-changed";

/// No credential is stored for the host: pairing has to be done again.
pub const NO_CREDENTIAL: &str = "no-credential";
/// The host id is not registered.
pub const UNKNOWN_HOST: &str = "unknown-host";

#[must_use]
pub(crate) fn error_response(
    status: StatusCode,
    message: &str,
) -> Response<Vec<u8>> {
    let mut response = Response::new(message.as_bytes().to_vec());
    *response.status_mut() = status;
    response
        .headers_mut()
        .insert(header::CONTENT_TYPE, HeaderValue::from_static("text/plain"));
    response.extensions_mut().insert(ProxyAnswer::Proxy);
    response
}

#[must_use]
pub(crate) fn marked_response(
    status: StatusCode,
    marker: &'static str,
    message: &str,
) -> Response<Vec<u8>> {
    let mut response = error_response(status, message);
    response
        .headers_mut()
        .insert(PROXY_HEADER, HeaderValue::from_static(marker));
    response
}

#[must_use]
pub(crate) fn readable_by(
    origin: Option<HeaderValue>,
    mut response: Response<Vec<u8>>,
) -> Response<Vec<u8>> {
    let Some(origin) = origin else {
        return response;
    };
    if response.extensions().get::<ProxyAnswer>().is_none() {
        return response;
    }
    let headers = response.headers_mut();
    headers.insert(header::ACCESS_CONTROL_ALLOW_ORIGIN, origin);
    headers.insert(
        header::ACCESS_CONTROL_EXPOSE_HEADERS,
        HeaderValue::from_static(PROXY_HEADER),
    );
    headers.append(header::VARY, HeaderValue::from_static("Origin"));
    response
}

/// The marker on `response`, if the proxy set one.
#[must_use]
pub fn marker_of(response: &Response<Vec<u8>>) -> Option<&str> {
    response
        .headers()
        .get(PROXY_HEADER)
        .and_then(|value| value.to_str().ok())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn only_the_proxys_own_answers_are_granted_to_the_page() {
        let page = HeaderValue::from_static("crowbar-pane://app");
        let own = readable_by(
            Some(page.clone()),
            marked_response(StatusCode::SERVICE_UNAVAILABLE, HOST_UNREACHABLE, "down"),
        );
        assert_eq!(
            own.headers().get(header::ACCESS_CONTROL_ALLOW_ORIGIN),
            Some(&page)
        );
        assert_eq!(own.headers().get(PROXY_HEADER).unwrap(), HOST_UNREACHABLE);

        let daemon = readable_by(Some(page), Response::new(Vec::new()));
        assert!(
            daemon
                .headers()
                .get(header::ACCESS_CONTROL_ALLOW_ORIGIN)
                .is_none()
        );
    }

    #[test]
    fn no_origin_means_no_cors_headers() {
        let answer = readable_by(None, error_response(StatusCode::BAD_GATEWAY, "x"));
        assert!(
            answer
                .headers()
                .get(header::ACCESS_CONTROL_ALLOW_ORIGIN)
                .is_none()
        );
    }
}
