//! The agents a host can start a chat with.

use serde_json::Value;

use crate::call::Call;
use crate::daemon_client::DaemonClient;
use crate::error::Result;
use crate::provider::Provider;
use crate::typed::typed;

const PATH: &str = "/v0/settings/chat/providers";

pub(crate) async fn list(client: &DaemonClient) -> Result<Vec<Provider>> {
    let data = client.send(Call::get(PATH)).await?;
    typed(PATH, data)
}

pub(crate) fn pick(providers: &[Provider]) -> Option<&Provider> {
    let enabled = || providers.iter().filter(|provider| provider.enabled.is_on());
    enabled()
        .find(|provider| provider.connected.is_on())
        .or_else(|| enabled().next())
}

pub(crate) fn fill(
    body: &mut Value,
    provider: &str,
) {
    if let Some(slot) = body.get_mut("provider") {
        *slot = Value::String(provider.to_owned());
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::switch::Switch;

    fn provider(
        id: &str,
        connected: Switch,
        enabled: Switch,
    ) -> Provider {
        Provider {
            id: id.into(),
            connected,
            enabled,
            ..Provider::default()
        }
    }

    #[test]
    fn a_connected_enabled_agent_wins_over_an_unconnected_one() {
        let all = [
            provider("a", Switch::Off, Switch::On),
            provider("b", Switch::On, Switch::On),
            provider("c", Switch::On, Switch::Off),
        ];
        assert_eq!(pick(&all).map(|p| p.id.as_str()), Some("b"));
        assert_eq!(pick(&all[..1]).map(|p| p.id.as_str()), Some("a"));
        assert!(pick(&all[2..]).is_none());
    }

    #[test]
    fn fill_sets_only_an_existing_provider_slot() {
        let mut body = json!({"provider": null});
        fill(&mut body, "claude");
        assert_eq!(body["provider"], "claude");
        let mut other = json!({"x": 1});
        fill(&mut other, "claude");
        assert_eq!(other, json!({"x": 1}));
    }
}
