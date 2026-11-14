use crate::extension_id::ExtensionId;
use crate::host_id::HostId;
use crate::pane_kind::PaneKind;

/// An HTTP request forwarded to a daemon host.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct DaemonRequest {
    /// The host the request is addressed to.
    pub host: HostId,
    /// The pane kind that issued it; extension origins are limited to their own routes.
    pub origin: PaneKind,
    /// The extension the issuing pane belongs to, for extension origins.
    pub extension: Option<ExtensionId>,
    /// HTTP method, for example `GET`.
    pub method: String,
    /// Path and query, for example `/v0/threads?limit=5`.
    pub path_and_query: String,
    /// Request headers.
    pub headers: Vec<(String, String)>,
    /// Request body.
    pub body: Vec<u8>,
}
