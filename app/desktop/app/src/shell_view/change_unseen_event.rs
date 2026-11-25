//! What a settings watcher logs when the shell closes before it hears of a change.

use std::fmt;

use journal::Level;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub(super) enum ChangeUnseenEvent {
    Appearance,
    Keybinding,
    PaneSetting,
}

impl fmt::Display for ChangeUnseenEvent {
    fn fmt(
        &self,
        f: &mut fmt::Formatter<'_>,
    ) -> fmt::Result {
        let change = match self {
            Self::Appearance => "an appearance change",
            Self::Keybinding => "a keybinding change",
            Self::PaneSetting => "a pane setting change",
        };
        write!(f, "the shell is closing; {change} goes unseen")
    }
}

impl journal::Event for ChangeUnseenEvent {
    fn source(&self) -> &'static str {
        match self {
            Self::Appearance => "crowbar::shell_view::appearance_settings",
            Self::Keybinding => "crowbar::shell_view::keymap_settings",
            Self::PaneSetting => "crowbar::shell_view::pane_settings_watch",
        }
    }

    fn name(&self) -> &'static str {
        match self {
            Self::Appearance => "appearance_change_unseen",
            Self::Keybinding => "keybinding_change_unseen",
            Self::PaneSetting => "pane_setting_change_unseen",
        }
    }

    fn level(&self) -> Level {
        Level::Debug
    }
}

#[cfg(test)]
mod tests {
    use journal::Event as _;

    use super::*;

    #[test]
    fn each_unseen_change_names_itself_and_reads_as_the_old_log_line() {
        let cases = [
            (
                ChangeUnseenEvent::Appearance,
                "appearance_change_unseen",
                "the shell is closing; an appearance change goes unseen",
            ),
            (
                ChangeUnseenEvent::Keybinding,
                "keybinding_change_unseen",
                "the shell is closing; a keybinding change goes unseen",
            ),
            (
                ChangeUnseenEvent::PaneSetting,
                "pane_setting_change_unseen",
                "the shell is closing; a pane setting change goes unseen",
            ),
        ];
        for (event, name, line) in cases {
            assert_eq!(event.name(), name);
            assert_eq!(event.level(), Level::Debug, "{name}");
            assert_eq!(event.to_string(), line, "{name}");
        }
    }
}
