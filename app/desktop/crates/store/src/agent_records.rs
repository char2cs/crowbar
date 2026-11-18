//! The Agents page's list, from the daemon's providers.

use daemonapi::Switch;
use serde_json::{Value, json};
use settings::{Line, Record, RecordFlag, Tone};

use crate::artwork;
use crate::provider::Provider;

pub(crate) const CONNECTED: &str = "connected";
pub(crate) const TOOLS: &str = "mcp";
pub(crate) const ENABLED: &str = "enabled";

pub(crate) fn of(providers: &[Provider]) -> Vec<Record> {
    providers.iter().map(record).collect()
}

fn record(provider: &Provider) -> Record {
    let record = Record::new(provider.id.as_str(), name_of(provider))
        .status(status_of(provider))
        .line(Line::plain(provider.id.as_str()).mono());
    with_artwork(record, provider)
        .flag(flag(CONNECTED, "Installed", provider.connected).as_dot())
        .flag(flag(TOOLS, "Tools", provider.mcp_enabled))
        .flag(flag(ENABLED, "Enabled", provider.enabled))
        .movable()
}

fn with_artwork(
    record: Record,
    provider: &Provider,
) -> Record {
    match artwork::vetted(&provider.icon) {
        Some(markup) => record.artwork(markup),
        None => record,
    }
}

fn flag(
    id: &str,
    label: &str,
    switch: Switch,
) -> RecordFlag {
    let flag = RecordFlag::new(id, label);
    match switch {
        Switch::On => flag.on(),
        Switch::Off => flag,
    }
}

fn name_of(provider: &Provider) -> &str {
    Some(provider.display_name.as_str())
        .filter(|name| !name.is_empty())
        .unwrap_or(&provider.id)
}

fn status_of(provider: &Provider) -> Tone {
    match provider.connected {
        Switch::On => Tone::Success,
        Switch::Off => Tone::Warning,
    }
}

pub(crate) fn preferences(providers: &[Provider]) -> Value {
    let list: Vec<Value> = providers
        .iter()
        .map(|provider| json!({"id": provider.id, "disabled": !provider.enabled.is_on(), "mcpDisabled": !provider.mcp_enabled.is_on()}))
        .collect();
    json!({"providers": list})
}

#[cfg(test)]
mod tests {
    use super::*;
    use daemonapi::Switch;

    fn provider(
        id: &str,
        enabled: Switch,
    ) -> Provider {
        Provider {
            id: id.into(),
            enabled,
            mcp_enabled: Switch::On,
            connected: Switch::On,
            ..Provider::default()
        }
    }

    #[test]
    fn each_provider_is_a_movable_record_and_the_body_lists_what_is_off() {
        let all = [
            provider("claude", Switch::On),
            provider("codex", Switch::Off),
        ];
        let records = of(&all);
        assert_eq!(
            records.iter().map(Record::id).collect::<Vec<_>>(),
            ["claude", "codex"]
        );
        assert_eq!(
            preferences(&all),
            json!({"providers": [
                {"id": "claude", "disabled": false, "mcpDisabled": false},
                {"id": "codex", "disabled": true, "mcpDisabled": false},
            ]})
        );
    }
}
