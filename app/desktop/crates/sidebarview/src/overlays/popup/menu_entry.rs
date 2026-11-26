//! One line of a context menu, as plain data.

use sidebar::Intent;
use ui::IconName;

use crate::overlays::popup::check::Check;
use crate::overlays::popup::tone::Tone;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum MenuEntry {
    Action {
        label: String,
        icon: Option<IconName>,
        tone: Tone,
        intent: Intent,
        checked: Option<Check>,
    },
    Separator,
    Submenu {
        label: String,
        icon: Option<IconName>,
        entries: Vec<MenuEntry>,
    },
}

impl MenuEntry {
    pub(crate) fn action(
        label: &str,
        icon: IconName,
        intent: Intent,
    ) -> Self {
        Self::Action {
            label: label.to_owned(),
            icon: Some(icon),
            tone: Tone::Normal,
            intent,
            checked: None,
        }
    }

    pub(crate) fn toggle(
        label: &str,
        icon: IconName,
        (check, intent): (Check, Intent),
    ) -> Self {
        Self::Action {
            label: label.to_owned(),
            icon: Some(icon),
            tone: Tone::Normal,
            intent,
            checked: Some(check),
        }
    }

    pub(crate) fn destructive(
        label: &str,
        icon: IconName,
        intent: Intent,
    ) -> Self {
        Self::Action {
            label: label.to_owned(),
            icon: Some(icon),
            tone: Tone::Destructive,
            intent,
            checked: None,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Command, RowId};

    #[test]
    fn only_a_destructive_entry_takes_the_destructive_tone() {
        let intent = Intent::Send(Command::Lock(RowId::from("r")));
        let tones = [
            MenuEntry::action("Lock", IconName::Lock, intent.clone()),
            MenuEntry::destructive("Delete", IconName::Trash, intent),
        ]
        .map(|entry| match entry {
            MenuEntry::Action { tone, .. } => Some(tone),
            _ => None,
        });
        assert_eq!(tones, [Some(Tone::Normal), Some(Tone::Destructive)]);
    }
}
