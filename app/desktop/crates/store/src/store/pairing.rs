//! Adding a host, the hosts paired so far, and the code a device pairs with.

pub(crate) mod device_offer;
pub(crate) mod local_dates;
pub(crate) mod offer;
pub(crate) mod offer_state;
pub(super) mod pairing_session;

use crate::live::HostSpec;
use crate::store::Store;

impl Store {
    /// The paired hosts being followed, in the order they were paired; this computer's daemon is
    /// not one of them.
    pub fn paired_hosts(&self) -> Vec<HostSpec> {
        self.hub.read(|state| {
            state
                .hosts
                .iter()
                .filter(|slot| slot.spec.endpoint.is_some())
                .map(|slot| slot.spec.clone())
                .collect()
        })
    }

    /// The id of the host that holds `project`; `None` when it is not loaded.
    pub fn project_host(
        &self,
        project: &str,
    ) -> Option<String> {
        self.hub
            .read(|state| crate::reads::host_of(state, project).map(|slot| slot.spec.id.clone()))
    }
}

#[cfg(test)]
mod tests {
    use crate::live::HostSpec;
    use crate::test_support::paired_store;

    const PROJECT: &str = r#"{"t":"entity","scope":"projects","kind":"project","id":"p","v":1,"data":{"id":"p","name":"Remote"}}"#;

    #[tokio::test]
    async fn the_paired_hosts_are_the_followed_remote_ones_and_hold_their_projects() {
        let data = paired_store("h1");
        let remote = HostSpec {
            id: "h1".into(),
            name: "h1 name".into(),
            endpoint: Some("127.0.0.1:4747".into()),
        };
        assert_eq!(data.paired_hosts(), vec![remote]);
        let frame = daemonapi::Frame::decode(PROJECT).unwrap();
        data.hub.edit(|state| state.apply("h1", frame));
        assert_eq!(data.project_host("p").as_deref(), Some("h1"));
        assert_eq!(data.project_host("gone"), None);
    }
}
