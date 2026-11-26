use crate::error::{Error, Result};
use crate::identity::HostId;
use crate::identity::PaneOrigin;
use crate::port::http_method::HttpMethod;
use crate::port::request_target::RequestTarget;

const MAX_HEADERS: usize = 128;

const MAX_HEADER_BYTES: usize = 64 * 1024;

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
    /// [`Error::InvalidRequest`] when there are more than 128 headers or their text
    /// passes 64 KiB.
    pub fn with_headers(
        mut self,
        headers: Vec<(String, String)>,
    ) -> Result<Self> {
        let bytes: usize = headers
            .iter()
            .map(|(name, value)| name.len() + value.len())
            .sum();
        if headers.len() > MAX_HEADERS || bytes > MAX_HEADER_BYTES {
            return Err(Error::invalid_request(
                "too many or too large request headers",
            ));
        }
        self.headers = headers;
        Ok(self)
    }

    /// Sets the request body.
    #[must_use]
    pub fn with_body(
        mut self,
        body: Vec<u8>,
    ) -> Self {
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
mod tests {
    use super::*;
    use crate::identity::ExtensionId;

    fn request() -> DaemonRequest {
        DaemonRequest::new(
            HostId::new("local").unwrap(),
            PaneOrigin::Extension(ExtensionId::new("git").unwrap()),
            HttpMethod::new("POST").unwrap(),
            RequestTarget::new("/v0/ext/git/x?a=1").unwrap(),
        )
    }

    #[test]
    fn header_count_and_size_are_capped() {
        let many = vec![("x".to_owned(), "v".to_owned()); MAX_HEADERS + 1];
        assert!(request().with_headers(many).is_err(), "too many");
        let big = vec![("x".to_owned(), "v".repeat(MAX_HEADER_BYTES))];
        assert!(request().with_headers(big).is_err(), "too large");
        let at_cap = vec![("x".to_owned(), "v".to_owned()); MAX_HEADERS];
        assert!(request().with_headers(at_cap).is_ok(), "at the cap");
    }
}
