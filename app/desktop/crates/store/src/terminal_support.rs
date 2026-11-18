//! Whether a host offers any agent that runs in a terminal.

use daemonapi::DaemonClient;

use crate::hub::Hub;
use crate::provider::Provider;
use crate::providers;

pub(crate) async fn read_terminal_support(
    hub: &Hub,
    host: &str,
    client: &DaemonClient,
) {
    let all = match providers::list(client).await {
        Ok(all) => all,
        Err(error) => {
            tracing::warn!(host, %error, "the host's agents could not be listed");
            return;
        },
    };
    let terminal = all.iter().any(Provider::offers_terminal);
    hub.edit(|state| state.set_terminal_threads(host, Some(terminal).into()));
}

#[cfg(test)]
mod tests {

    use super::*;
    use crate::test_support::state;

    #[tokio::test]
    async fn a_host_whose_agents_cannot_be_listed_stays_unknown() {
        let (state, _) = state();
        let client = state.hosts[0].client.clone();
        let hub = Hub::new(state, crate::test_support::quiet());
        read_terminal_support(&hub, "local", &client).await;
        assert!(!hub.read(|state| state.hosts[0].terminal_threads));
    }
}
