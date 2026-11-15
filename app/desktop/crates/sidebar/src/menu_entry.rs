//! One line of a context menu, as plain data.

use ui::IconName;

use crate::intent::Intent;
use crate::tone::Tone;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum MenuEntry {
    Action {
        label: String,
        icon: Option<IconName>,
        tone: Tone,
        intent: Intent,
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
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::command::Command;
    use crate::row_id::RowId;

    #[test]
    fn constructors_set_the_tone() {
        let intent = Intent::Send(Command::Lock(RowId::from("r")));
        let normal = MenuEntry::action("Lock", IconName::Lock, intent.clone());
        let destructive = MenuEntry::destructive("Delete", IconName::Trash, intent);
        assert!(
            matches!(
                normal,
                MenuEntry::Action {
                    tone: Tone::Normal,
                    ..
                }
            ),
            "normal"
        );
        assert!(
            matches!(
                destructive,
                MenuEntry::Action {
                    tone: Tone::Destructive,
                    ..
                }
            ),
            "destructive"
        );
    }
}
