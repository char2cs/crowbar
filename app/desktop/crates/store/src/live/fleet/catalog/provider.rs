//! The part of a provider record that decides what a new chat may do.

use serde::Deserialize;

use daemonapi::Switch;

#[derive(Debug, Clone, PartialEq, Eq, Default, Deserialize)]
#[serde(rename_all = "camelCase", default)]
pub(crate) struct Provider {
    pub(crate) id: String,
    pub(crate) display_name: String,
    pub(crate) icon: String,
    pub(crate) mcp_enabled: Switch,
    pub(crate) connected: Switch,
    pub(crate) enabled: Switch,
    pub(crate) has_terminal: Switch,
    pub(crate) terminal_start_here: Switch,
}

impl Provider {
    pub(crate) fn offers_terminal(&self) -> bool {
        self.enabled.is_on() && self.has_terminal.is_on()
    }

    pub(crate) fn lands_on_terminal(&self) -> bool {
        self.offers_terminal() && self.terminal_start_here.is_on()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const CASES: [(&str, bool, bool); 5] = [
        (
            r#"{"enabled":true,"hasTerminal":true,"terminalStartHere":true}"#,
            true,
            true,
        ),
        (r#"{"enabled":true,"hasTerminal":true}"#, true, false),
        (r#"{"enabled":true,"terminalStartHere":true}"#, false, false),
        (r#"{"enabled":false,"hasTerminal":true}"#, false, false),
        (r"{}", false, false),
    ];

    #[test]
    fn a_provider_offers_a_terminal_when_enabled_and_able_and_lands_there_when_it_may_start_there()
    {
        for (json, offers, lands) in CASES {
            let provider: Provider = serde_json::from_str(json).unwrap();
            let seen = (provider.offers_terminal(), provider.lands_on_terminal());
            assert_eq!(seen, (offers, lands), "{json}");
        }
    }
}
