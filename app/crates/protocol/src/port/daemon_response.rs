#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DaemonResponse {
    status: u16,
    headers: Vec<(String, String)>,
    body: Vec<u8>,
}

impl DaemonResponse {
    #[must_use]
    pub fn new(
        status: u16,
        headers: Vec<(String, String)>,
        body: Vec<u8>,
    ) -> Self {
        Self {
            status,
            headers,
            body,
        }
    }

    #[must_use]
    pub fn status(&self) -> u16 {
        self.status
    }

    #[must_use]
    pub fn headers(&self) -> &[(String, String)] {
        &self.headers
    }

    #[must_use]
    pub fn body(&self) -> &[u8] {
        &self.body
    }

    #[must_use]
    pub fn proxy_marker(&self) -> Option<&str> {
        self.headers
            .iter()
            .find(|(name, _)| name.eq_ignore_ascii_case(crate::PROXY_HEADER))
            .map(|(_, value)| value.as_str())
    }

    #[must_use]
    pub fn into_body(self) -> Vec<u8> {
        self.body
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_proxy_answer_names_its_marker_and_a_daemon_answer_none() {
        let proxied = DaemonResponse::new(
            503,
            vec![("X-Crowbar-Proxy".into(), crate::HOST_UNREACHABLE.into())],
            Vec::new(),
        );
        let daemon = DaemonResponse::new(503, Vec::new(), Vec::new());
        assert_eq!(proxied.proxy_marker(), Some(crate::HOST_UNREACHABLE));
        assert_eq!(daemon.proxy_marker(), None);
    }
}
