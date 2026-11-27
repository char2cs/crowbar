use crate::scheme::protocol_request::ProtocolRequest;
use crate::scheme::protocol_response::ProtocolResponse;

pub type ProtocolResponder = Box<dyn FnOnce(ProtocolResponse) + Send + 'static>;

pub trait ProtocolHandler: Send + Sync + 'static {
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
    use crate::view::Id;

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
                web_view: Id(1),
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
