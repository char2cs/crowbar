use std::path::PathBuf;
use std::sync::Arc;

use crate::protocol_handler::ProtocolHandler;

/// Settings shared by every web view of a host.
#[derive(Clone)]
pub struct HostConfig {
    /// The custom scheme served by `protocol`, without `://`, for example `crowbar`.
    pub scheme: String,
    /// Serves requests to the scheme.
    pub protocol: Arc<dyn ProtocolHandler>,
    /// Where web contexts keep cookies and storage; `None` keeps the engine default.
    pub data_dir: Option<PathBuf>,
}

impl std::fmt::Debug for HostConfig {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("HostConfig")
            .field("scheme", &self.scheme)
            .field("data_dir", &self.data_dir)
            .finish_non_exhaustive()
    }
}
