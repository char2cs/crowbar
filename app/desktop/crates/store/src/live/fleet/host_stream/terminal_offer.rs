use crate::live::fleet::catalog as providers;
use crate::live::fleet::catalog::Provider;
use crate::live::state::Hub;

pub(crate) async fn read_terminal_support(
    hub: &Hub,
    host: &str,
    client: &daemonapi::Client,
) {
    let all = match providers::list(client).await {
        Ok(all) => all,
        Err(error) => {
            tracing::warn!(host, %error, "the host's agents could not be listed");
            return;
        },
    };
    let terminal = all.iter().any(Provider::offers_terminal);
    let icons = all
        .into_iter()
        .map(|provider| (provider.id, provider.icon))
        .collect();
    hub.edit(|state| {
        state.set_terminal_threads(host, Some(terminal).into());
        state.set_provider_icons(host, icons);
    });
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
