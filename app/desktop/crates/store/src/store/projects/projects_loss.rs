//! A lost projects snapshot on demand, as a stream that drops mid-subscription loses it.

use crate::live::State;
use crate::store::Store;

impl Store {
    /// Loses this computer's projects snapshot once, as a stream that drops before `synced` does.
    pub fn lose_projects_snapshot(&self) {
        self.hub.edit(lose);
    }
}

fn lose(state: &mut State) {
    let Some(host) = local_host(state) else {
        return;
    };
    state.begin_snapshot(&host);
    state.lose_snapshot(&host);
}

fn local_host(state: &State) -> Option<String> {
    state
        .hosts
        .iter()
        .find(|slot| slot.spec.endpoint.is_none())
        .map(|slot| slot.spec.id.clone())
}

#[cfg(test)]
mod tests {
    use crate::store::ProjectsLoad;
    use crate::test_support::local_store;

    #[tokio::test]
    async fn two_lost_snapshots_fail_the_projects_as_a_stream_dropping_twice_does() {
        let data = local_store(1);
        data.lose_projects_snapshot();
        assert_eq!(data.projects_load(), ProjectsLoad::Loading, "one loss");
        data.lose_projects_snapshot();
        assert_eq!(data.projects_load(), ProjectsLoad::Failed, "two in a row");
    }
}
