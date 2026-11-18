//! Creating and killing sessions through the shell's transport.

mod binding;
mod session_base;

pub(crate) use binding::Binding;
pub use session_base::SessionBase;

use std::sync::Arc;

use protocol::{
    DaemonRequest, DaemonResponse, DaemonTransport, HostId, HttpMethod, PaneOrigin, RequestTarget,
    Responder,
};
use serde_json::Value;
use tokio::sync::oneshot;

use crate::error::{Error, Result};

#[derive(Clone)]
pub(crate) struct Rest {
    pub(crate) transport: Arc<dyn DaemonTransport>,
    pub(crate) host: HostId,
}

impl Rest {
    pub(crate) async fn create(
        &self,
        base: &SessionBase,
    ) -> Result<String> {
        let path = base.create_path();
        let response = self.send("POST", path).await?;
        let data = daemonapi::envelope::unwrap(path, &response)?;
        let id = data.pointer("/sessionId").and_then(Value::as_str);
        id.map(str::to_owned).ok_or(Error::NoSessionId)
    }

    pub(crate) async fn kill(
        &self,
        base: &SessionBase,
        session: &str,
    ) -> Result<()> {
        let path = base.kill_path(session);
        let response = self.send("DELETE", &path).await?;
        daemonapi::envelope::unwrap(&path, &response)?;
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
    use std::sync::Mutex;

    use protocol::{RelayEndpoint, RelayScope};

    use super::*;

    const CREATED: u16 = 201;

    struct Canned {
        status: u16,
        body: &'static str,
        seen: Mutex<Vec<String>>,
    }

    impl DaemonTransport for Canned {
        fn request(
            &self,
            request: DaemonRequest,
            respond: Responder,
        ) {
            let line = format!(
                "{} {}",
                request.method().as_str(),
                request.target().as_str()
            );
            self.seen.lock().unwrap().push(line);
            respond(Ok(DaemonResponse::new(
                self.status,
                Vec::new(),
                self.body.as_bytes().to_vec(),
            )));
        }

        fn relay_endpoint(
            &self,
            _scope: &RelayScope,
        ) -> protocol::Result<RelayEndpoint> {
            Err(protocol::Error::Timeout)
        }
    }

    fn rest(
        status: u16,
        body: &'static str,
    ) -> (Rest, Arc<Canned>) {
        let canned = Arc::new(Canned {
            status,
            body,
            seen: Mutex::default(),
        });
        let transport: Arc<dyn DaemonTransport> = Arc::<Canned>::clone(&canned);
        let host = HostId::new("local").unwrap();
        (Rest { transport, host }, canned)
    }

    #[tokio::test]
    async fn creating_posts_to_the_chat_and_reads_the_session_id() {
        let (rest, canned) = rest(CREATED, r#"{"success":true,"data":{"sessionId":"s9"}}"#);
        let base = SessionBase::chat("c1").unwrap();
        assert_eq!(rest.create(&base).await.unwrap(), "s9");
        assert_eq!(
            *canned.seen.lock().unwrap(),
            ["POST /v0/chats/c1/terminals"]
        );
    }

    const FAILURES: [(u16, &str); 3] = [
        (CREATED, r#"{"success":true,"data":{}}"#),
        (404, ""),
        (
            409,
            r#"{"success":false,"error":"the chat has no terminal"}"#,
        ),
    ];
    const EXPECTED: [&str; 3] = [
        "terminal session: the daemon answered without a session id",
        "terminal session: the daemon answered 404",
        "terminal session: the chat has no terminal",
    ];

    #[tokio::test]
    async fn refusals_and_empty_answers_are_errors() {
        assert_eq!(create_errors().await, EXPECTED);
    }

    async fn create_errors() -> Vec<String> {
        let mut errors = Vec::new();
        for (status, body) in FAILURES {
            let (rest, _) = rest(status, body);
            let base = SessionBase::chat("c1").unwrap();
            errors.push(rest.create(&base).await.unwrap_err().to_string());
        }
        errors
    }
}
