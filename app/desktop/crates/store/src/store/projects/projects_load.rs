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
    use daemonapi::Frame;

    use super::*;
    use crate::test_support::local_store;

    #[derive(Debug, Clone, Copy, PartialEq, Eq)]
    enum Step {
        Lose,
        Retry,
        Sync,
    }

    fn lose(state: &mut State) {
        state.begin_snapshot("local");
        state.lose_snapshot("local");
    }

    fn sync(state: &mut State) {
        let synced = Frame::Synced {
            scope: "projects".into(),
            unread: Vec::new(),
        };
        state.apply("local", synced);
    }

    fn take(
        data: &Store,
        step: Step,
    ) {
        match step {
            Step::Lose => data.hub.edit(lose),
            Step::Retry => data.retry_projects(),
            Step::Sync => data.hub.edit(sync),
        }
    }

    fn load_after(steps: &[Step]) -> ProjectsLoad {
        let data = local_store(1);
        for step in steps {
            take(&data, *step);
        }
        data.projects_load()
    }

    #[tokio::test]
    async fn a_snapshot_lost_twice_in_a_row_fails_until_retried_or_synced() {
        use Step::{Lose, Retry, Sync};
        let cases: [(&[Step], ProjectsLoad); 5] = [
            (&[], ProjectsLoad::Loading),
            (&[Lose], ProjectsLoad::Loading),
            (&[Lose, Lose], ProjectsLoad::Failed),
            (&[Lose, Lose, Retry], ProjectsLoad::Loading),
            (&[Lose, Lose, Sync], ProjectsLoad::Loaded),
        ];
        let seen = cases.map(|(steps, _)| (steps, load_after(steps)));
        assert_eq!(seen, cases);
        assert_eq!(local_store(0).projects_load(), ProjectsLoad::Loaded);
    }
}
