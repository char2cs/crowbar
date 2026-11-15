use crate::protocol_request::ProtocolRequest;
use crate::protocol_response::ProtocolResponse;

/// Completion callback of [`ProtocolHandler::handle`]; call it exactly once, from any thread.
pub type ProtocolResponder = Box<dyn FnOnce(ProtocolResponse) + Send + 'static>;

/// Serves the custom scheme of a [`WebViewHost`](crate::WebViewHost).
///
/// Called on the platform thread; implementations hand work to their own executor and answer
/// through the responder.
pub trait ProtocolHandler: Send + Sync + 'static {
    /// Handles one request.
    fn handle(
        &self,
        request: ProtocolRequest,
        respond: ProtocolResponder,
    );
}

#[cfg(test)]
mod tests {
    use std::sync::mpsc;

    use super::*;
    use crate::web_view_id::WebViewId;

    struct Echo;

    impl ProtocolHandler for Echo {
        fn handle(
            &self,
            request: ProtocolRequest,
            respond: ProtocolResponder,
        ) {
            respond(ProtocolResponse {
                status: 200,
                headers: Vec::new(),
                body: request.uri.into_bytes(),
            });
        }
    }

    #[test]
    fn a_handler_answers_through_its_responder() {
        let (sender, receiver) = mpsc::channel();
        let handler: &dyn ProtocolHandler = &Echo;
        handler.handle(
            ProtocolRequest {
                web_view: WebViewId(1),
                uri: "crowbar://local/x".into(),
                method: "GET".into(),
                headers: Vec::new(),
                body: Vec::new(),
            },
            Box::new(move |response| sender.send(response).unwrap()),
        );
        assert_eq!(
            receiver.recv().unwrap().body,
            b"crowbar://local/x",
            "echoed"
        );
    }
}
