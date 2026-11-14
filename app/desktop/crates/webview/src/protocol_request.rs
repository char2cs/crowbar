use crate::web_view_id::WebViewId;

/// A request a page made to the host's custom scheme, for example `crowbar://local/v0/health`.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProtocolRequest {
    /// The web view that issued it. This is the caller's identity; the page cannot forge it.
    pub web_view: WebViewId,
    /// The full request URI.
    pub uri: String,
    /// HTTP method.
    pub method: String,
    /// Request headers.
    pub headers: Vec<(String, String)>,
    /// Request body.
    pub body: Vec<u8>,
}
