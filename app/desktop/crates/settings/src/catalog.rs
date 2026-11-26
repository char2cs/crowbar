//! The pages that ship with the shell.

mod agents;
mod appearance;
mod browser;
mod editor;
mod files;
mod git;
mod keybindings;
mod machines;
mod rows;
mod search_index;
mod sessions;
mod terminal;

use crate::schema::Schema;
use crate::values::Error;

/// Source name of the agents list, to be answered by a [`crate::RecordBackend`].
pub const AGENT_RECORDS: &str = agents::AGENTS;
/// Source name of the descriptor checks list.
pub const DESCRIPTOR_RECORDS: &str = agents::DESCRIPTORS;
/// Source name of the active sessions list.
pub const SESSION_RECORDS: &str = sessions::SESSIONS;
/// Source name of the hosts list.
pub const HOST_RECORDS: &str = machines::hosts::HOSTS;
/// Source name of the paired devices list.
pub const DEVICE_RECORDS: &str = machines::devices::DEVICES;
/// Whether other devices may connect to this computer's daemon, which stores it.
pub const DEVICE_CONNECTIONS: &str = machines::devices::ALLOW;
/// The address the daemon listens on for devices.
pub const DEVICE_ADDRESS: &str = machines::devices::ADDRESS;
/// Where devices reach the daemon while connections are allowed.
pub const DEVICE_LISTENING: &str = machines::devices::LISTENING;
/// The terminal line height develop once defaulted to, read on load as today's default.
pub const LEGACY_TERMINAL_LINE_HEIGHT: crate::schema::LegacyNumber = terminal::LEGACY_LINE_HEIGHT;
/// The setting that stores custom keybindings.
pub const KEYBINDING_OVERRIDES: &str = keybindings::OVERRIDES;
/// The setting that stores the active keybinding preset.
pub const KEYBINDING_PRESET: &str = keybindings::PRESET;

/// The chords the preset `preset` binds, keyed by command id.
pub fn preset_overrides(preset: &str) -> std::collections::BTreeMap<String, String> {
    keybindings::preset_overrides(preset)
}

/// The eleven pages of the shell: Appearance, Editor, Files, Git, Terminal, Browser, Agents,
/// Active sessions, Keybindings, Hosts and Devices, in rail order.
///
/// # Errors
/// [`Error::Duplicate`] when the built-in pages repeat an id or key, which the tests rule out.
pub fn default_schema() -> Result<Schema, Error> {
    Schema::new(vec![
        appearance::page(),
        editor::page(),
        files::page(),
        git::page(),
        terminal::page(),
        browser::page(),
        agents::page(),
        sessions::page(),
        keybindings::page(),
        machines::hosts::page(),
        machines::devices::page(),
    ])
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::schema::ChoiceSource;
    use crate::schema::Control;
    use crate::schema::Row;
    use crate::test_support::{Case, run_cases};
    use crate::values::Choice;
    use crate::values::Value;

    fn eleven_pages_in_rail_order_with_unique_keys() {
        let schema = default_schema().unwrap();
        let ids: Vec<&str> = schema
            .pages()
            .iter()
            .map(|page| page.id().as_str())
            .collect();
        assert_eq!(
            ids,
            [
                "appearance",
                "editor",
                "file-explorer",
                "git",
                "terminal",
                "browser",
                "providers",
                "active-sessions",
                "keybindings",
                "hosts",
                "devices",
            ]
        );
    }

    fn default_fits(row: &Row) -> bool {
        let Some(default) = &row.default else {
            return true;
        };
        match (&row.control, default) {
            (Control::Number { range, .. }, Value::Number(n)) => {
                (range.min..=range.max).contains(n)
            },
            (Control::Switch { .. }, Value::Bool(_))
            | (Control::Text { .. } | Control::Select { .. }, Value::Text(_))
            | (Control::TextList { .. }, Value::List(_)) => true,
            _ => false,
        }
    }

    fn every_default_matches_the_shape_its_control_stores() {
        let schema = default_schema().unwrap();
        let misfits: Vec<&str> = schema
            .rows()
            .filter(|row| !default_fits(row))
            .map(|row| row.key().as_str())
            .collect();
        assert!(misfits.is_empty(), "{misfits:?}");
    }

    fn fixed_options(row: &Row) -> Option<(&[Choice], &str)> {
        let (
            Control::Select {
                source: ChoiceSource::Fixed(choices),
                ..
            },
            Some(Value::Text(default)),
        ) = (&row.control, &row.default)
        else {
            return None;
        };
        Some((choices.as_slice(), default))
    }

    fn fixed_select_defaults_are_among_the_options() {
        let schema = default_schema().unwrap();
        let strays: Vec<&str> = schema
            .rows()
            .filter_map(|row| Some((row, fixed_options(row)?)))
            .filter(|(_, (choices, default))| !choices.iter().any(|c| c.value() == *default))
            .map(|(row, _)| row.key().as_str())
            .collect();
        assert!(strays.is_empty(), "{strays:?}");
    }

    fn every_page_icon_is_a_known_phosphor_stem() {
        let schema = default_schema().unwrap();
        let stems = [
            "paint-brush",
            "code-block",
            "tree-structure",
            "git-branch",
            "terminal-window",
            "globe",
            "robot",
            "pulse",
            "keyboard",
            "hard-drives",
            "device-mobile",
        ];
        for page in schema.pages() {
            assert!(stems.contains(&page.icon.as_str()), "{}", page.icon);
        }
    }

    const CASES: &[Case] = &[
        (
            "eleven pages in rail order with unique keys",
            eleven_pages_in_rail_order_with_unique_keys,
        ),
        (
            "every default matches the shape its control stores",
            every_default_matches_the_shape_its_control_stores,
        ),
        (
            "fixed select defaults are among the options",
            fixed_select_defaults_are_among_the_options,
        ),
        (
            "every page icon is a known phosphor stem",
            every_page_icon_is_a_known_phosphor_stem,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
