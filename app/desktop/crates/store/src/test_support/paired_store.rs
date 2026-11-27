use std::sync::Arc;

use crate::live::HostSpec;
use crate::store::Ports;
use crate::store::Store;

use super::{FakeDaemon, desk, quiet};

pub(crate) fn paired_store(id: &str) -> Store {
    let transport: Arc<dyn protocol::DaemonTransport> = FakeDaemon::shared();
    let local = HostSpec {
        id: "local".into(),
        name: "This computer".into(),
        endpoint: None,
    };
    let paired = HostSpec {
        id: id.into(),
        name: format!("{id} name"),
        endpoint: Some("127.0.0.1:4747".into()),
    };
    let wiring = Ports {
        desk: desk(),
        signal: quiet(),
        outcome: Arc::new(|_| ()),
    };
    Store::start(
        &tokio::runtime::Handle::current(),
        &transport,
        vec![local, paired],
        wiring,
    )
}
