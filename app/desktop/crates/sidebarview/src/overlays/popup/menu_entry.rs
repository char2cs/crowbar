//! One line of a context menu, as plain data.

use sidebar::Intent;

use crate::overlays::popup::check::Check;

#[derive(Debug, Clone, PartialEq, Eq)]
pub(crate) enum MenuEntry {
    Action {
        label: String,
        intent: Intent,
        checked: Option<Check>,
    },
    Separator,
    Submenu {
        label: String,
        entries: Vec<MenuEntry>,
    },
}

impl MenuEntry {
    pub(crate) fn action(
        label: &str,
        intent: Intent,
    ) -> Self {
        Self::Action {
            label: label.to_owned(),
            intent,
            checked: None,
        }
    }

    pub(crate) fn toggle(
        label: &str,
        (check, intent): (Check, Intent),
    ) -> Self {
        Self::Action {
            label: label.to_owned(),
            intent,
            checked: Some(check),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Command, RowId};

    #[test]
    fn only_a_toggle_carries_a_check() {
        let intent = Intent::Send(Command::Lock(RowId::from("r")));
        let plain = MenuEntry::action("Lock", intent.clone());
        let toggle = MenuEntry::toggle("Hidden", (Check::On, intent));
        assert!(matches!(plain, MenuEntry::Action { checked: None, .. }));
        assert!(matches!(
            toggle,
            MenuEntry::Action {
                checked: Some(Check::On),
                ..
            }
        ));
    }
}
