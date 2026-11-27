mod binding;
mod session_paths;

pub use binding::Binding;
pub use session_paths::SessionPaths;

use std::sync::Arc;

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, HostId, HttpMethod, PaneOrigin, RequestTarget,
    Responder,
};
use serde_json::Value;
use tokio::sync::oneshot;

use crate::error::{Error, Result};

const ENDED: &str = "ended";

#[derive(Clone)]
pub(crate) struct Rest {
    pub(crate) transport: Arc<dyn DaemonTransport>,
    pub(crate) host: HostId,
}

impl Rest {
    pub(crate) async fn create(
        &self,
        base: &SessionPaths,
    ) -> Result<String> {
        let path = base.create_path();
        let response = self.send("POST", path).await?;
        let data = daemonapi::Envelope::unwrap(path, &response)?;
        let id = data.pointer("/sessionId").and_then(Value::as_str);
        id.map(str::to_owned).ok_or(Error::NoSessionId)
    }

    pub(crate) async fn live(
        &self,
        base: &SessionPaths,
    ) -> Result<Vec<String>> {
        let path = base.create_path();
        let response = self.send("GET", path).await?;
        let data = daemonapi::Envelope::unwrap(path, &response)?;
        let sessions = data.as_array().map_or(&[][..], Vec::as_slice);
        Ok(sessions.iter().filter_map(live_id).collect())
    }

    pub(crate) async fn kill(
        &self,
        base: &SessionPaths,
        session: &str,
    ) -> Result<()> {
        let path = base.kill_path(session);
        let response = self.send("DELETE", &path).await?;
        daemonapi::Envelope::unwrap(&path, &response)?;
        Ok(())
    }

    async fn send(
        &self,
        method: &str,
        path: &str,
    ) -> Result<DaemonResponse> {
        let target = RequestTarget::new(path).map_err(daemon_error)?;
        let method = HttpMethod::new(method).map_err(daemon_error)?;
        let request = DaemonRequest::new(self.host.clone(), PaneOrigin::Core, method, target);
        let (respond, answer) = oneshot::channel();
        self.transport.request(request, reply(respond));
        answer
            .await
            .map_err(Error::TransportStopped)?
            .map_err(daemon_error)
    }
}

fn live_id(session: &Value) -> Option<String> {
    let ended = session.get("status").and_then(Value::as_str) == Some(ENDED);
    let id = session.get("id").and_then(Value::as_str)?;
    (!ended).then(|| id.to_owned())
}

fn reply(respond: oneshot::Sender<protocol::Result<DaemonResponse>>) -> Responder {
    Box::new(move |result| deliver(respond, result))
}

fn deliver(
    respond: oneshot::Sender<protocol::Result<DaemonResponse>>,
    result: protocol::Result<DaemonResponse>,
) {
    if respond.send(result).is_err() {
        tracing::debug!("a terminal request was abandoned");
    }
}

fn daemon_error(error: impl std::fmt::Display) -> Error {
    Error::Daemon(error.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{CREATED, daemon_answering, daemon_creating, rest};

    #[tokio::test]
    async fn creating_posts_to_the_chat_and_reads_the_session_id() {
        let daemon = daemon_creating(CREATED, r#"{"success":true,"data":{"sessionId":"s9"}}"#);
        let base = SessionPaths::chat("c1").unwrap();
        assert_eq!(rest(&daemon).create(&base).await.unwrap(), "s9");
        assert_eq!(
            *daemon.seen.lock().unwrap(),
            ["POST /v0/chats/c1/terminals"]
        );
    }

    #[tokio::test]
    async fn the_live_sessions_are_the_listed_ones_that_have_not_ended() {
        let body = r#"{"success":true,"data":[{"id":"s1","status":"running"},{"id":"s2","status":"ended"},{"id":"s3","status":"suspended"}]}"#;
        let daemon = daemon_answering(CREATED, body);
        let base = SessionPaths::chat("c1").unwrap();
        assert_eq!(rest(&daemon).live(&base).await.unwrap(), ["s1", "s3"]);
        assert_eq!(*daemon.seen.lock().unwrap(), ["GET /v0/chats/c1/terminals"]);
    }

    const FAILURE_CASES: [(u16, &str, &str); 3] = [
        (
            CREATED,
            r#"{"success":true,"data":{}}"#,
            "terminal session: the daemon answered without a session id",
        ),
        (404, "", "terminal session: the daemon answered 404"),
        (
            409,
            r#"{"success":false,"error":"the chat has no terminal"}"#,
            "terminal session: the chat has no terminal",
        ),
    ];

    #[tokio::test]
    async fn refusals_and_empty_answers_are_errors() {
        check_all().await;
    }

    async fn check_all() {
        for (status, body, expected) in FAILURE_CASES {
            let base = SessionPaths::chat("c1").unwrap();
            let error = rest(&daemon_creating(status, body)).create(&base).await;
            assert_eq!(error.unwrap_err().to_string(), expected);
        }
    }
}
