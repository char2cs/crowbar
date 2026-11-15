//! Requests to one daemon host through the shell's transport.

use std::sync::Arc;

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, HostId, HttpMethod, PaneOrigin, RequestTarget,
    Responder,
};
use serde_json::Value;
use tokio::sync::oneshot;

use crate::call::Call;
use crate::envelope;
use crate::error::{Error, Result};

#[derive(Clone)]
pub(crate) struct DaemonClient {
    transport: Arc<dyn DaemonTransport>,
    host: HostId,
}

impl std::fmt::Debug for DaemonClient {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter
            .debug_struct("DaemonClient")
            .field("host", &self.host)
            .finish_non_exhaustive()
    }
}

impl DaemonClient {
    pub(crate) fn new(
        transport: Arc<dyn DaemonTransport>,
        host: HostId,
    ) -> Self {
        Self { transport, host }
    }

    pub(crate) async fn send(
        &self,
        call: Call,
    ) -> Result<Value> {
        let path = call.path.clone();
        let response = self.raw(call).await.map_err(|source| Error::Request {
            path: path.clone(),
            source,
        })?;
        envelope::unwrap(&path, &response)
    }

    pub(crate) async fn bytes(
        &self,
        call: Call,
    ) -> Result<Vec<u8>> {
        let path = call.path.clone();
        let response = self.raw(call).await.map_err(|source| Error::Request {
            path: path.clone(),
            source,
        })?;
        match response.status() {
            status if status >= envelope::FIRST_ERROR_STATUS => Err(refused_with(path, status)),
            _ => Ok(response.body().to_vec()),
        }
    }

    async fn raw(
        &self,
        call: Call,
    ) -> protocol::Result<DaemonResponse> {
        let request = self.build(call)?;
        let (respond, answer) = oneshot::channel();
        self.transport.request(request, reply(respond));
        answer.await.unwrap_or(Err(protocol::Error::Interrupted))
    }

    fn build(
        &self,
        call: Call,
    ) -> protocol::Result<DaemonRequest> {
        let Call {
            method,
            path,
            body,
            raw,
            mut headers,
        } = call;
        let target = RequestTarget::new(path)?;
        let mut request = DaemonRequest::new(
            self.host.clone(),
            PaneOrigin::Core,
            HttpMethod::new(method)?,
            target,
        );
        if let Some(body) = body {
            headers.push(("content-type".into(), "application/json".into()));
            request = request.with_body(body.to_string().into_bytes());
        }
        if let Some(raw) = raw {
            request = request.with_body(raw);
        }
        request.with_headers(headers)
    }
}

fn refused_with(
    path: String,
    status: u16,
) -> Error {
    Error::Refused {
        path,
        status,
        code: None,
        message: format!("the daemon answered {status}"),
    }
}

fn reply(respond: oneshot::Sender<protocol::Result<DaemonResponse>>) -> Responder {
    Box::new(move |result| deliver(respond, result))
}

fn deliver(
    respond: oneshot::Sender<protocol::Result<DaemonResponse>>,
    result: protocol::Result<DaemonResponse>,
) {
    if respond.send(result).is_err() {
        tracing::debug!("the caller stopped waiting for a daemon answer");
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::{FakeDaemon, local};

    #[tokio::test]
    async fn a_call_reaches_the_route_with_its_body_and_headers() {
        let daemon = FakeDaemon::shared();
        daemon.answer(("GET", "/img"), 200, "rawbytes");
        daemon.ok("POST", "/v0/projects", r#"{"id":"p"}"#);
        let client = DaemonClient::new(Arc::<FakeDaemon>::clone(&daemon), local());
        let data = client
            .send(
                Call::post("/v0/projects")
                    .json(json!({"name": "A"}))
                    .idempotent("k"),
            )
            .await
            .unwrap();
        assert_eq!(data, json!({"id": "p"}));
        let raw = client.bytes(Call::get("/img")).await.unwrap();
        assert_eq!(raw, b"rawbytes");
        let seen = daemon.seen();
        assert_eq!(seen[0].body, Some(json!({"name": "A"})));
        assert!(
            seen[0]
                .headers
                .contains(&("idempotency-key".into(), "k".into()))
        );
    }

    #[tokio::test]
    async fn an_unknown_route_is_a_refusal_and_a_bad_path_is_a_request_error() {
        let client = DaemonClient::new(FakeDaemon::shared(), local());
        let refused = client.send(Call::get("/v0/none")).await.unwrap_err();
        assert!(
            matches!(refused, Error::Refused { status: 404, .. }),
            "{refused}"
        );
        let bad = client.send(Call::get("no-slash")).await.unwrap_err();
        assert!(matches!(bad, Error::Request { .. }), "{bad}");
        let none = client.bytes(Call::get("/v0/none")).await.unwrap_err();
        assert!(matches!(none, Error::Refused { status: 404, .. }), "{none}");
    }
}
