use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, Error, RelayEndpoint, RelayScope, Responder,
    Result,
};

/// A daemon that only answers `GET /v0/health`. It stands in until the real transport is
/// wired and is what `--loopback` runs against, so the shell can be exercised without a daemon.
#[derive(Debug, Default)]
pub(crate) struct LoopbackDaemon;

impl DaemonTransport for LoopbackDaemon {
    fn request(&self, request: DaemonRequest, respond: Responder) {
        let healthy = request.method == "GET" && request.path_and_query == "/v0/health";
        let (status, body) = if healthy {
            (200, r#"{"status":"ok"}"#)
        } else {
            (404, r#"{"error":"loopback daemon serves only /v0/health"}"#)
        };
        respond(Ok(DaemonResponse {
            status,
            headers: vec![("content-type".into(), "application/json".into())],
            body: body.as_bytes().to_vec(),
        }));
    }

    fn relay_endpoint(&self, _scope: &RelayScope) -> Result<RelayEndpoint> {
        Err(Error::HostUnavailable("the loopback daemon has no stream relay".into()))
    }
}

#[cfg(test)]
#[path = "loopback_daemon_tests.rs"]
mod tests;
