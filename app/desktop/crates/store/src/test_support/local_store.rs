use std::sync::Arc;

use crate::live::HostSpec;
use crate::store::Ports;
use crate::store::Store;

use super::{FakeDaemon, desk, quiet};

/// A store following this computer's daemon (or none), started on the current runtime.
pub(crate) fn local_store(hosts: usize) -> Store {
    let transport: Arc<dyn protocol::DaemonTransport> = FakeDaemon::shared();
    let spec = HostSpec {
        id: "local".into(),
        name: "This computer".into(),
        endpoint: None,
    };
    let wiring = Ports {
        desk: desk(),
        signal: quiet(),
        outcome: Arc::new(|_| ()),
    };
    let specs = std::iter::repeat_n(spec, hosts).collect();
    Store::start(
        &tokio::runtime::Handle::current(),
        &transport,
        specs,
        wiring,
    )
}
