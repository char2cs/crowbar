/// The daemon's answer to a [`DaemonRequest`](crate::DaemonRequest).
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DaemonResponse {
    /// HTTP status code.
    pub status: u16,
    /// Response headers.
    pub headers: Vec<(String, String)>,
    /// Response body, bytes as sent by the daemon.
    pub body: Vec<u8>,
}
