//! A followed host as the shell holds it: a slot of the hub and the dial of its stream.

use std::sync::Arc;

use daemonapi::{CORE_ORIGIN, Dial, RelayDial};
use protocol::{DaemonTransport, HostId};

use crate::live::held::HostSlot;
use crate::live::held::HostSpec;

pub(crate) type Dialled = (HostSpec, Arc<dyn Dial>);

pub(crate) fn addressable(text: &str) -> Option<HostId> {
    HostId::new(text)
        .inspect_err(|error| tracing::warn!(host = text, %error, "a host id the transport cannot address is skipped"))
        .ok()
}

pub(crate) fn relay_for(
    transport: &Arc<dyn DaemonTransport>,
    spec: HostSpec,
) -> Option<Dialled> {
    let id = addressable(&spec.id)?;
    let dial: Arc<dyn Dial> = Arc::new(RelayDial::new(Arc::clone(transport), id, CORE_ORIGIN));
    Some((spec, dial))
}

pub(crate) fn slot_for(
    transport: &Arc<dyn DaemonTransport>,
    spec: HostSpec,
) -> Option<HostSlot> {
    let id = addressable(&spec.id)?;
    let client = daemonapi::Client::new(Arc::clone(transport), id);
    Some(HostSlot::new(spec, client))
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::FakeDaemon;

    fn spec(id: &str) -> HostSpec {
        HostSpec {
            id: id.to_owned(),
            name: id.to_owned(),
            endpoint: Some("box:3737".to_owned()),
        }
    }

    #[test]
    fn a_host_the_transport_can_address_gets_a_dial_and_a_slot_and_another_neither() {
        let transport: Arc<dyn DaemonTransport> = FakeDaemon::shared();
        assert!(relay_for(&transport, spec("lab")).is_some());
        assert!(slot_for(&transport, spec("lab")).is_some());
        assert!(relay_for(&transport, spec("Bad Id")).is_none());
        assert!(slot_for(&transport, spec("Bad Id")).is_none());
    }
}
