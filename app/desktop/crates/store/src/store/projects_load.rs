//! Whether this computer's projects have loaded, as develop's shell landing tells it.

use crate::live::State;
use crate::store::Store;

const PERSISTENT: u32 = 2;

/// How the projects of this computer's daemon have loaded.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ProjectsLoad {
    /// They are listed, or there is no daemon of this computer to list them.
    Loaded,
    /// The first snapshot is still coming.
    Loading,
    /// The snapshot was lost twice in a row; only Retry or a reconnect asks again.
    Failed,
}

impl Store {
    /// How the projects of this computer's daemon have loaded.
    pub fn projects_load(&self) -> ProjectsLoad {
        self.hub.read(load_of)
    }

    /// Asks this computer's daemon for its projects again, forgetting the failed attempts.
    pub fn retry_projects(&self) {
        let host = self.hub.edit(|state| {
            let slot = state
                .hosts
                .iter_mut()
                .find(|slot| slot.spec.endpoint.is_none())?;
            slot.snapshot_failures = 0;
            Some(slot.spec.id.clone())
        });
        if let Some(host) = host {
            self.fleet
                .steer(Some(&host), crate::live::StreamControl::Resubscribe);
        }
    }
}

fn load_of(state: &State) -> ProjectsLoad {
    let Some(slot) = state.hosts.iter().find(|slot| slot.spec.endpoint.is_none()) else {
        return ProjectsLoad::Loaded;
    };
    if slot.store.synced() {
        return ProjectsLoad::Loaded;
    }
    if slot.snapshot_failures >= PERSISTENT {
        return ProjectsLoad::Failed;
    }
    ProjectsLoad::Loading
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::live::Entities;
    use crate::test_support::state;

    fn lost(times: usize) -> ProjectsLoad {
        let (mut state, _daemon) = state();
        state.hosts[0].store = Entities::default();
        for _ in 0..times {
            state.begin_snapshot("local");
            state.lose_snapshot("local");
        }
        load_of(&state)
    }

    #[test]
    fn a_snapshot_lost_twice_in_a_row_is_a_failed_load() {
        let cases = [
            (0, ProjectsLoad::Loading),
            (1, ProjectsLoad::Loading),
            (2, ProjectsLoad::Failed),
        ];
        for (times, load) in cases {
            assert_eq!(lost(times), load, "{times} lost");
        }
        let (synced, _daemon) = state();
        assert_eq!(load_of(&synced), ProjectsLoad::Loaded);
    }
}
