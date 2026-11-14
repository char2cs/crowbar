use crate::error::{Error, Result};
use crate::host_id::HostId;
use crate::http_method::HttpMethod;
use crate::pane_origin::PaneOrigin;
use crate::request_target::RequestTarget;

/// Most headers a request carries.
pub const MAX_HEADERS: usize = 128;

/// Largest header block of a request, in bytes (names plus values).
pub const MAX_HEADER_BYTES: usize = 64 * 1024;

/// An HTTP request forwarded to a daemon host.
///
/// Every field is validated at construction, so the target cannot change the host the request
/// is addressed to and an extension origin always names its extension.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DaemonRequest {
    host: HostId,
    origin: PaneOrigin,
    method: HttpMethod,
    target: RequestTarget,
    headers: Vec<(String, String)>,
    body: Vec<u8>,
}

impl DaemonRequest {
    /// A request without headers or body from `origin` to `host`.
    #[must_use]
    pub fn new(
        host: HostId,
        origin: PaneOrigin,
        method: HttpMethod,
        target: RequestTarget,
    ) -> Self {
        Self {
            host,
            origin,
            method,
            target,
            headers: Vec::new(),
            body: Vec::new(),
        }
    }

    /// Sets the request headers.
    ///
    /// # Errors
    /// [`Error::InvalidRequest`] when there are more than [`MAX_HEADERS`] headers or their text
    /// passes [`MAX_HEADER_BYTES`].
    pub fn with_headers(mut self, headers: Vec<(String, String)>) -> Result<Self> {
        let bytes: usize = headers
            .iter()
            .map(|(name, value)| name.len() + value.len())
            .sum();
        if headers.len() > MAX_HEADERS || bytes > MAX_HEADER_BYTES {
            return Err(Error::InvalidRequest {
                reason: "too many or too large request headers",
            });
        }
        self.headers = headers;
        Ok(self)
    }

    /// Sets the request body.
    #[must_use]
    pub fn with_body(mut self, body: Vec<u8>) -> Self {
        self.body = body;
        self
    }

    /// The host the request is addressed to.
    #[must_use]
    pub fn host(&self) -> &HostId {
        &self.host
    }

    /// The pane origin that issued it; extension origins are limited to their own routes.
    #[must_use]
    pub fn origin(&self) -> &PaneOrigin {
        &self.origin
    }

    /// The HTTP method.
    #[must_use]
    pub fn method(&self) -> &HttpMethod {
        &self.method
    }

    /// The path and query, for example `/v0/threads?limit=5`.
    #[must_use]
    pub fn target(&self) -> &RequestTarget {
        &self.target
    }

    /// The request headers.
    #[must_use]
    pub fn headers(&self) -> &[(String, String)] {
        &self.headers
    }

    /// The request body.
    #[must_use]
    pub fn body(&self) -> &[u8] {
        &self.body
    }

    /// Takes the body out, so a forwarder can move it instead of copying it.
    #[must_use]
    pub fn into_body(self) -> Vec<u8> {
        self.body
    }
}

#[cfg(test)]
#[path = "daemon_request_tests.rs"]
mod tests;
