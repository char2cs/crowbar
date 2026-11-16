use std::collections::BTreeMap;

use protocol::{CommandId, KeyBinding, KeyChord};

use crate::keystroke::Keystroke;
use crate::shell_command::ShellCommand;
use crate::shell_keys::project_bindings;

/// The chords bound to shell commands: the defaults with the user's overrides applied.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Keymap {
    bound: BTreeMap<String, ShellCommand>,
}

impl Keymap {
    /// The default chords of every shell command.
    #[must_use]
    pub fn defaults() -> Self {
        Self::with_overrides(std::iter::empty::<(&str, &str)>())
    }

    /// The defaults with `overrides` applied. Each override is a `(command id, chord)` pair; an
    /// empty chord unbinds the command, an unknown command or an unusable chord is ignored, and
    /// a chord taken by an override frees the default command that held it.
    #[must_use]
    pub fn with_overrides<I, C, K>(overrides: I) -> Self
    where
        I: IntoIterator<Item = (C, K)>,
        C: AsRef<str>,
        K: AsRef<str>,
    {
        let overrides: BTreeMap<ShellCommand, String> = overrides
            .into_iter()
            .filter_map(|(id, chord)| {
                let command = ShellCommand::parse(id.as_ref())?;
                Some((command, chord.as_ref().trim().to_lowercase()))
            })
            .collect();
        let bound = ShellCommand::all()
            .map(|command| {
                let chord = overrides
                    .get(&command)
                    .map_or_else(|| command.default_chord().to_owned(), Clone::clone);
                (command, chord)
            })
            .filter(|(_, chord)| !chord.is_empty())
            .collect::<Vec<_>>();
        Self {
            bound: claim_chords(bound, &overrides),
        }
    }

    /// The shell command a key press runs, if any.
    #[must_use]
    pub fn resolve(
        &self,
        stroke: &Keystroke,
    ) -> Option<ShellCommand> {
        self.bound.get(&stroke.chord()?).copied()
    }

    /// The chord of `command`, for menus and the palette.
    #[must_use]
    pub fn chord_of(
        &self,
        command: ShellCommand,
    ) -> Option<&str> {
        self.bound
            .iter()
            .find(|(_, bound)| **bound == command)
            .map(|(chord, _)| chord.as_str())
    }

    /// The bindings a pane forwards to the shell as `command.run`.
    #[must_use]
    pub fn forwarded(&self) -> Vec<KeyBinding> {
        self.bound
            .iter()
            .filter_map(|(chord, command)| {
                let keys = KeyChord::new(chord.as_str()).ok()?;
                let id = CommandId::new(command.id()).ok()?;
                Some(KeyBinding::new(keys, id))
            })
            .chain(project_bindings())
            .collect()
    }
}

fn claim_chords(
    wanted: Vec<(ShellCommand, String)>,
    overrides: &BTreeMap<ShellCommand, String>,
) -> BTreeMap<String, ShellCommand> {
    let (chosen, defaults): (Vec<_>, Vec<_>) = wanted
        .into_iter()
        .partition(|(command, _)| overrides.contains_key(command));
    chosen
        .into_iter()
        .chain(defaults)
        .fold(BTreeMap::new(), |mut bound, (command, chord)| {
            bound.entry(chord).or_insert(command);
            bound
        })
}

#[cfg(test)]
mod tests {
    use super::*;

    fn stroke(chord: &str) -> Keystroke {
        let mut parts: Vec<&str> = chord.split('+').collect();
        let key = parts.pop().unwrap_or_default().to_owned();
        Keystroke {
            primary: parts.contains(&"mod"),
            shift: parts.contains(&"shift"),
            alt: parts.contains(&"alt"),
            key,
        }
    }

    #[test]
    fn defaults_resolve_typed_chords() {
        let keymap = Keymap::defaults();
        let cases = [
            ("mod+t", Some(ShellCommand::NewTab)),
            ("mod+shift+t", Some(ShellCommand::ReopenClosedTab)),
            ("mod+w", Some(ShellCommand::CloseTab)),
            ("mod+shift+p", Some(ShellCommand::CommandPalette)),
            ("mod+s", None),
            ("t", None),
        ];
        for (chord, expected) in cases {
            assert_eq!(keymap.resolve(&stroke(chord)), expected, "{chord}");
        }
    }

    #[test]
    fn an_override_moves_a_command() {
        let keymap = Keymap::with_overrides([("tabs.new", "mod+shift+k")]);
        assert_eq!(keymap.resolve(&stroke("mod+t")), None);
        assert_eq!(
            keymap.resolve(&stroke("mod+shift+k")),
            Some(ShellCommand::NewTab)
        );
        assert_eq!(keymap.chord_of(ShellCommand::NewTab), Some("mod+shift+k"));
    }

    #[test]
    fn an_empty_override_unbinds_and_bad_ones_are_ignored() {
        let keymap = Keymap::with_overrides([("tabs.new", ""), ("editor.save", "mod+s")]);
        assert_eq!(keymap.chord_of(ShellCommand::NewTab), None);
        assert_eq!(keymap.resolve(&stroke("mod+s")), None);
    }

    #[test]
    fn a_chosen_chord_frees_the_default_that_held_it() {
        let keymap = Keymap::with_overrides([("agent.newChat", "mod+t")]);
        assert_eq!(
            keymap.resolve(&stroke("mod+t")),
            Some(ShellCommand::NewChat)
        );
        assert_eq!(keymap.chord_of(ShellCommand::NewTab), None);
    }

    #[test]
    fn forwarded_bindings_cover_the_bound_shell_commands_and_the_project_slots() {
        let bindings = Keymap::with_overrides([("tabs.new", "")]).forwarded();
        let commands: Vec<&str> = bindings.iter().map(|b| b.command().as_str()).collect();
        assert!(commands.contains(&"tabs.closeActive"));
        assert!(!commands.contains(&"tabs.new"));
        assert!(commands.contains(&"navigation.selectProject.1"));
        assert!(bindings.iter().all(|b| !b.keys().as_str().contains(' ')));
    }
}
