use tokio_tungstenite::client_async;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;

use crate::error::Result;
use crate::journal_sink::emit;
use crate::rest::SessionPaths;
use crate::stream::Attachment;
use crate::stream::End;
use crate::stream::Reconnect;
use crate::stream::pump;
use crate::stream::recover;
use crate::stream::{header, stream_error, within};
use crate::terminal_sessions::TerminalSessions;
use crate::terminal_sessions::terminal_session_event::TerminalSessionEvent;

const RELAY_SUBPROTOCOL: &str = "crowbar.relay.v1";
const TOKEN_PREFIX: &str = "crowbar.token.";

impl TerminalSessions {
    pub(super) async fn keep_attached(
        &self,
        target: (&SessionPaths, &str),
        attachment: &Attachment,
    ) -> End {
        let mut reconnect = Reconnect::default();
        let mut ended = None;
        while ended.is_none() {
            ended = self.round(target, attachment, &mut reconnect).await;
        }
        ended.unwrap_or(End::Lost)
    }

    async fn round(
        &self,
        target: (&SessionPaths, &str),
        attachment: &Attachment,
        reconnect: &mut Reconnect,
    ) -> Option<End> {
        let outcome = self.stream(target, attachment).await;
        let reason = match outcome {
            Ok(End::Lost) => {
                reconnect.connected();
                "the connection dropped".to_owned()
            },
            Ok(end) => return Some(end),
            Err(error) => error.to_string(),
        };
        emit(&TerminalSessionEvent::StreamLost { reason: &reason });
        if !recover(&attachment.ends, reconnect, &reason).await {
            return Some(End::Lost);
        }
        self.unlisted(target).await
    }

    async fn unlisted(
        &self,
        (base, session): (&SessionPaths, &str),
    ) -> Option<End> {
        let live = self.rest.live(base).await.ok()?;
        (!live.iter().any(|listed| listed == session)).then_some(End::Gone)
    }

    async fn stream(
        &self,
        (base, session): (&SessionPaths, &str),
        attachment: &Attachment,
    ) -> Result<End> {
        let endpoint = self
            .rest
            .transport
            .relay_endpoint(&attachment.scope)
            .map_err(stream_error)?;
        let address = endpoint.url().trim_start_matches("ws://").to_owned();
        let tcp = within((self.dial)(address)).await?;
        let url = format!("{}{}", endpoint.url(), base.stream_path(session));
        let mut request = url.into_client_request().map_err(stream_error)?;
        let protocols = format!("{RELAY_SUBPROTOCOL}, {TOKEN_PREFIX}{}", endpoint.token());
        let headers = request.headers_mut();
        headers.insert("sec-websocket-protocol", header(&protocols)?);
        headers.insert("origin", header(&self.origin)?);
        let (socket, _) = within(client_async(request, tcp)).await?;
        Ok(pump(socket, attachment.ends.clone()).await)
    }
}

#[cfg(test)]
mod tests {
    use crate::report::Report;
    use crate::rest::Binding;
    use crate::test_support::{
        CREATED, NONE_LISTED, View, all_reports, daemon_answering, memory_dial, serve_then_drop,
        sessions,
    };

    #[tokio::test]
    async fn a_session_the_daemon_stops_listing_while_its_stream_is_down_is_gone() {
        let (dial, accepted) = memory_dial();
        let daemon = daemon_answering(CREATED, NONE_LISTED);
        let server = tokio::spawn(serve_then_drop(accepted));
        let view = View::open(&sessions(&daemon, dial), Binding::Create);
        let told = all_reports(&view.reports).await;
        server.abort();
        assert_eq!(told, [Report::Bound("s1".into()), Report::Gone]);
        let seen = daemon.seen.lock().unwrap().clone();
        assert_eq!(
            seen,
            ["POST /v0/chats/c1/terminals", "GET /v0/chats/c1/terminals"],
            "not killed"
        );
    }
}
