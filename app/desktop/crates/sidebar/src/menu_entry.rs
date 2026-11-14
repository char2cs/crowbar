//! One line of a context menu, as plain data.

use ui::IconName;

use crate::intent::Intent;
use crate::tone::Tone;

/// An action, a separator or a submenu. The view turns these into popup rows.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum MenuEntry {
    /// A row that applies an intent when chosen.
    Action {
        /// The text.
        label: String,
        /// The leading icon.
        icon: Option<IconName>,
        /// Normal or destructive.
        tone: Tone,
        /// What choosing it does.
        intent: Intent,
    },
    /// A hairline between groups.
    Separator,
    /// A row that opens a nested list to the right.
    Submenu {
        /// The text.
        label: String,
        /// The leading icon.
        icon: Option<IconName>,
        /// The nested entries.
        entries: Vec<MenuEntry>,
    },
}

impl MenuEntry {
    pub(crate) fn action(label: &str, icon: IconName, intent: Intent) -> Self {
        Self::Action {
            label: label.to_owned(),
            icon: Some(icon),
            tone: Tone::Normal,
            intent,
        }
    }

    pub(crate) fn destructive(label: &str, icon: IconName, intent: Intent) -> Self {
        Self::Action {
            label: label.to_owned(),
            icon: Some(icon),
            tone: Tone::Destructive,
            intent,
        }
    }
}

#[cfg(test)]
#[path = "menu_entry_tests.rs"]
mod tests;
