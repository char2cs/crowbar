/// The answer to a [`ProtocolRequest`](crate::ProtocolRequest).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ProtocolResponse {
    /// HTTP status code.
    pub status: u16,
    /// Response headers.
    pub headers: Vec<(String, String)>,
    /// Response body.
    pub body: Vec<u8>,
}
