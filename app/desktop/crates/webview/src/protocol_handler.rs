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
    fn handle(&self, request: ProtocolRequest, respond: ProtocolResponder);
}
