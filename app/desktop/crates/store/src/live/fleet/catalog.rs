//! The agents a host can start a chat with.

mod provider;
mod surface;

pub(crate) use provider::Provider;
pub(crate) use surface::Surface;

use daemonapi::{Call, decode::typed};
use serde_json::Value;

use crate::error::Result;

const PATH: &str = "/v0/settings/chat/providers";

pub(crate) async fn list(client: &daemonapi::Client) -> Result<Vec<Provider>> {
    let data = client.send(Call::get(PATH)).await?;
    Ok(typed(PATH, data)?)
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

pub(crate) fn land(
    body: &mut Value,
    (provider, landing): (&Provider, Surface),
) {
    let terminal = landing == Surface::Terminal && provider.lands_on_terminal();
    if let Some(fields) = body.as_object_mut().filter(|_| terminal) {
        fields.insert("surface".into(), Value::String("terminal".into()));
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use daemonapi::Switch;

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
    fn a_terminal_landing_is_set_only_for_a_provider_that_starts_there() {
        let able = Provider {
            enabled: Switch::On,
            has_terminal: Switch::On,
            terminal_start_here: Switch::On,
            ..Provider::default()
        };
        let cases = [
            (&able, Surface::Terminal, Some("terminal")),
            (&able, Surface::Default, None),
            (&Provider::default(), Surface::Terminal, None),
        ];
        for (provider, landing, expected) in cases {
            let mut body = json!({"id": "n"});
            land(&mut body, (provider, landing));
            assert_eq!(body.get("surface").and_then(Value::as_str), expected);
        }
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
