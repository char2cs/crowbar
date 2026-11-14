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

/// A plain-text answer the proxy wrote itself rather than relayed from a daemon.
#[must_use]
pub fn error_response(status: u16, message: &str) -> Response<Vec<u8>> {
    let mut response = Response::new(message.as_bytes().to_vec());
    *response.status_mut() = StatusCode::from_u16(status).unwrap_or(StatusCode::BAD_GATEWAY);
    response
        .headers_mut()
        .insert(header::CONTENT_TYPE, HeaderValue::from_static("text/plain"));
    response.extensions_mut().insert(ProxyAnswer);
    response
}

/// [`error_response`] carrying a [`PROXY_HEADER`] marker naming the failure.
#[must_use]
pub fn marked_response(status: u16, marker: &'static str, message: &str) -> Response<Vec<u8>> {
    let mut response = error_response(status, message);
    response
        .headers_mut()
        .insert(PROXY_HEADER, HeaderValue::from_static(marker));
    response
}

/// The page reaches `crowbar://` cross-origin, so WebKit hides any answer without CORS headers
/// (status and marker too) and `fetch` rejects opaquely. A daemon grants its own answers; the
/// proxy's are granted here.
#[must_use]
pub fn readable_by(
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
#[path = "response_tests.rs"]
mod tests;
