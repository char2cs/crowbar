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
}

#[cfg(test)]
mod tests {
    use crate::live::HostSpec;
    use crate::test_support::paired_store;

    #[tokio::test]
    async fn the_paired_hosts_are_the_followed_remote_ones() {
        let data = paired_store("h1");
        let remote = HostSpec {
            id: "h1".into(),
            name: "h1 name".into(),
            endpoint: Some("127.0.0.1:4747".into()),
        };
        assert_eq!(data.paired_hosts(), vec![remote]);
    }
}
