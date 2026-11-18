//! The host a picker follows: who it is and how to reach it.

use daemonapi::DaemonClient;

use crate::sidebar_reads::{active_project, host_of};
use crate::state::State;

#[derive(Debug, Clone)]
pub(crate) struct Followed {
    pub(crate) id: String,
    pub(crate) name: String,
    pub(crate) client: DaemonClient,
}

pub(crate) fn named(
    state: &State,
    host: &str,
) -> Option<Followed> {
    let slot = state.hosts.iter().find(|slot| slot.spec.id == host)?;
    Some(Followed {
        id: slot.spec.id.clone(),
        name: slot.spec.name.clone(),
        client: slot.client.clone(),
    })
}

pub(crate) fn of_open_space(state: &State) -> Option<Followed> {
    let project = active_project(state)?;
    let slot = host_of(state, project.as_str())?;
    named(state, &slot.spec.id)
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn a_followed_host_is_found_by_id_and_the_open_space_names_its_own() {
        let (state, _) = state();
        assert_eq!(named(&state, "local").unwrap().name, "This computer");
        assert!(named(&state, "ghost").is_none());
        assert_eq!(of_open_space(&state).unwrap().id, "local");
    }
}
