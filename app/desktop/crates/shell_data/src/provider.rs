//! The part of a provider record that decides what a new chat may do.

use serde::Deserialize;

use crate::switch::Switch;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct Provider {
    pub(crate) id: String,
    pub(crate) display_name: String,
    pub(crate) mcp_enabled: Switch,
    pub(crate) connected: Switch,
    pub(crate) enabled: Switch,
    pub(crate) has_terminal: Switch,
}

impl Provider {
    pub(crate) fn offers_terminal(&self) -> bool {
        self.enabled.is_on() && self.has_terminal.is_on()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn provider(json: &str) -> Provider {
        serde_json::from_str(json).unwrap()
    }

    const CASES: [(&str, bool); 4] = [
        (r#"{"enabled":true,"hasTerminal":true}"#, true),
        (r#"{"enabled":true,"hasTerminal":false}"#, false),
        (r#"{"enabled":false,"hasTerminal":true}"#, false),
        (r"{}", false),
    ];

    #[test]
    fn a_provider_offers_a_terminal_only_when_enabled_and_able() {
        for (json, offers) in CASES {
            assert_eq!(provider(json).offers_terminal(), offers, "{json}");
        }
    }
}
