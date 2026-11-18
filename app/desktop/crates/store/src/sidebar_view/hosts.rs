//! The hosts the sidebar lists, as the state holds them.

use sidebar::{Host, HostId, HostKind};

use crate::live::HostSlot;
use crate::live::State;

pub(crate) fn hosts(state: &State) -> Vec<Host> {
    state.hosts.iter().map(host_of_slot).collect()
}

fn host_of_slot(slot: &HostSlot) -> Host {
    Host {
        id: HostId::from(slot.spec.id.as_str()),
        name: slot.spec.name.clone(),
        kind: host_kind(slot),
        state: slot.link.host_state(),
        endpoint: slot.spec.endpoint.clone(),
    }
}

fn host_kind(slot: &HostSlot) -> HostKind {
    if slot.spec.endpoint.is_some() {
        return HostKind::Remote;
    }
    HostKind::Local
}

#[cfg(test)]
mod tests {
    use sidebar::HostState;

    use super::*;
    use crate::test_support::state;

    #[test]
    fn each_followed_host_is_listed_with_its_kind_and_link() {
        let (mut state, _) = state();
        state.hosts[0].spec.endpoint = None;
        let local = hosts(&state);
        assert_eq!(
            (local[0].name.as_str(), local[0].state, local[0].kind),
            ("This computer", HostState::Probing, HostKind::Local)
        );
        state.hosts[0].spec.endpoint = Some("box:3737".into());
        assert_eq!(hosts(&state)[0].kind, HostKind::Remote);
    }
}
