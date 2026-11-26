//! Chords the shell takes only while their surface shows, and only from a page that left the
//! key alone: Escape on the sidebar drawer and Mod+F on the Files tree. They are fixed, never
//! rebound, as in develop.

use chord::Keystroke;
use protocol::{CommandId, KeyBinding, KeyChord, Precedence};

/// A command whose chord applies only while its surface shows.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash, PartialOrd, Ord)]
pub enum ContextualCommand {
    /// Close the sidebar lying over the content as a drawer (Escape).
    DismissDrawer,
    /// Open the Files tree's search (Mod+F).
    SearchFiles,
}

const ALL: [ContextualCommand; 2] = [
    ContextualCommand::DismissDrawer,
    ContextualCommand::SearchFiles,
];

impl ContextualCommand {
    /// The command with id `id`, as a page's `command.run` names it.
    #[must_use]
    pub fn parse(id: &str) -> Option<Self> {
        ALL.into_iter().find(|command| command.id() == id)
    }

    /// The command a key press asks for, whether or not its surface shows.
    #[must_use]
    pub fn of_stroke(stroke: &Keystroke) -> Option<Self> {
        ALL.into_iter().find(|command| command.matches(stroke))
    }

    /// The binding a page forwards while the surface shows; the page's own handling wins.
    #[must_use]
    pub fn binding(self) -> Option<KeyBinding> {
        let keys = KeyChord::new(self.chord()).ok()?;
        let id = CommandId::new(self.id()).ok()?;
        Some(KeyBinding::new(keys, id).with_precedence(Precedence::Page))
    }

    fn id(self) -> &'static str {
        match self {
            Self::DismissDrawer => "sidebar.dismissDrawer",
            Self::SearchFiles => "files.search",
        }
    }

    fn chord(self) -> &'static str {
        match self {
            Self::DismissDrawer => "escape",
            Self::SearchFiles => "mod+f",
        }
    }

    fn matches(
        self,
        stroke: &Keystroke,
    ) -> bool {
        stroke.chord().as_deref() == Some(self.chord())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::stroke;

    #[test]
    fn each_command_answers_its_fixed_chord_and_is_forwarded_page_first() {
        let cases = [
            (ContextualCommand::DismissDrawer, "escape"),
            (ContextualCommand::SearchFiles, "mod+f"),
        ];
        for (command, pressed) in cases {
            assert_eq!(
                ContextualCommand::of_stroke(&stroke(pressed)),
                Some(command)
            );
            let binding = command.binding().unwrap();
            assert_eq!(binding.precedence(), Precedence::Page, "{command:?}");
            assert_eq!(
                ContextualCommand::parse(binding.command().as_str()),
                Some(command),
                "{command:?}"
            );
        }
        assert_eq!(ContextualCommand::of_stroke(&stroke("shift+f")), None);
    }
}
