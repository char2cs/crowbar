use std::path::PathBuf;
use std::sync::Arc;

use crate::scheme::ProtocolHandler;

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
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("HostConfig")
            .field("scheme", &self.scheme)
            .field("data_dir", &self.data_dir)
            .finish_non_exhaustive()
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::scheme::ProtocolRequest;
    use crate::scheme::ProtocolResponder;

    struct Silent;

    impl ProtocolHandler for Silent {
        fn handle(
            &self,
            _request: ProtocolRequest,
            _respond: ProtocolResponder,
        ) {
        }
    }

    #[test]
    fn debug_shows_the_scheme_and_hides_the_handler() {
        let config = HostConfig {
            scheme: "crowbar".into(),
            protocol: Arc::new(Silent),
            data_dir: None,
        };
        let text = format!("{config:?}");
        assert!(text.contains("crowbar"), "scheme shown: {text}");
        assert!(!text.contains("Silent"), "handler hidden: {text}");
    }
}
