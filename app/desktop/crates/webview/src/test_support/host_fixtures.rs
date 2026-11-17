use std::sync::Arc;
use std::sync::atomic::{AtomicUsize, Ordering};

use crate::host_config::HostConfig;
use crate::protocol_handler::{ProtocolHandler, ProtocolResponder};
use crate::protocol_request::ProtocolRequest;

struct Silent {
    ignored: AtomicUsize,
}

impl ProtocolHandler for Silent {
    fn handle(
        &self,
        _request: ProtocolRequest,
        _respond: ProtocolResponder,
    ) {
        self.ignored.fetch_add(1, Ordering::Relaxed);
    }
}

pub(crate) fn silent_host_config() -> HostConfig {
    HostConfig {
        scheme: "crowbar".into(),
        protocol: Arc::new(Silent {
            ignored: AtomicUsize::new(0),
        }),
        data_dir: None,
    }
}
