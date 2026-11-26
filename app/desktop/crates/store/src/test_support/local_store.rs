use std::sync::Arc;

use crate::live::HostSpec;
use crate::store::Ports;
use crate::store::Store;

use super::{FakeDaemon, desk, quiet};

fn this_computer() -> HostSpec {
    HostSpec {
        id: "local".into(),
        name: "This computer".into(),
        endpoint: None,
    }
}

fn started(specs: Vec<HostSpec>) -> Store {
    let transport: Arc<dyn protocol::DaemonTransport> = FakeDaemon::shared();
    let wiring = Ports {
        desk: desk(),
        signal: quiet(),
        outcome: Arc::new(|_| ()),
    };
    Store::start(
        &tokio::runtime::Handle::current(),
        &transport,
        specs,
        wiring,
    )
}

/// A store following this computer's daemon (or none), started on the current runtime.
pub(crate) fn local_store(hosts: usize) -> Store {
    started(std::iter::repeat_n(this_computer(), hosts).collect())
}

/// A store following this computer's daemon and the paired host `id`, started on the current
/// runtime.
pub(crate) fn paired_store(id: &str) -> Store {
    let paired = HostSpec {
        id: id.into(),
        name: format!("{id} name"),
        endpoint: Some("127.0.0.1:4747".into()),
    };
    started(vec![this_computer(), paired])
}
