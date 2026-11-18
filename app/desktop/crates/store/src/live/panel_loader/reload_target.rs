//! The directory listing to load again once a change to the workspace's files has run.

use crate::live::panel_loader::load_directory;
use crate::live::state::Hub;

pub(crate) struct ReloadTarget {
    pub(crate) client: Option<daemonapi::Client>,
    pub(crate) chat: String,
    pub(crate) directory: String,
}

impl ReloadTarget {
    pub(crate) async fn load(
        self,
        hub: &Hub,
    ) {
        if let Some(client) = self.client {
            load_directory(hub, &client, (&self.chat, &self.directory)).await;
        }
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::test_support::state;

    #[tokio::test]
    async fn without_a_client_nothing_is_loaded() {
        let (state, _) = state();
        let hub = Hub::new(state, Arc::new(|| ()));
        let target = ReloadTarget {
            client: None,
            chat: "hc".into(),
            directory: "src".into(),
        };
        target.load(&hub).await;
        assert!(hub.read(|state| state.panels.listings.is_empty()));
    }
}
