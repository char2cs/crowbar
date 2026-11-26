use std::collections::BTreeMap;

use protocol::{CommandId, KeyBinding, KeyChord};

use crate::contextual_command::ContextualCommand;
use crate::project_slot::project_bindings;
use crate::shell_command::ShellCommand;
use chord::Keystroke;

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

    /// The shell command a key press runs instead of the focused view, if any.
    #[must_use]
    pub fn resolve(
        &self,
        stroke: &Keystroke,
    ) -> Option<ShellCommand> {
        self.bound_to(stroke)
            .filter(|command| !command.runs_alongside())
    }

    /// The shell command a key press runs beside the focused view's own handling, if any: chat
    /// zoom, which a focused terminal also takes for its font.
    #[must_use]
    pub fn alongside(
        &self,
        stroke: &Keystroke,
    ) -> Option<ShellCommand> {
        self.bound_to(stroke)
            .filter(|command| command.runs_alongside())
    }

    fn bound_to(
        &self,
        stroke: &Keystroke,
    ) -> Option<ShellCommand> {
        self.bound.get(&stroke.chord()?).copied()
    }

    /// The bindings a pane forwards to the shell as `command.run`, with the `live` contextual
    /// commands whose surface shows now.
    #[must_use]
    pub fn forwarded(
        &self,
        live: &[ContextualCommand],
    ) -> Vec<KeyBinding> {
        self.bound
            .iter()
            .filter_map(|(chord, command)| {
                let keys = KeyChord::new(chord.as_str()).ok()?;
                let id = CommandId::new(command.id()).ok()?;
                Some(KeyBinding::new(keys, id))
            })
            .chain(project_bindings())
            .chain(live.iter().filter_map(|command| command.binding()))
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
    use crate::test_support::stroke;

    type Runs = Option<ShellCommand>;
    type Override = (&'static str, &'static str);

    #[test]
    fn defaults_resolve_typed_chords_and_chat_zoom_runs_alongside() {
        const CASES: &[(&str, Runs, Runs)] = &[
            ("mod+t", Some(ShellCommand::NewTab), None),
            ("mod+shift+t", Some(ShellCommand::ReopenClosedTab), None),
            ("mod+w", Some(ShellCommand::CloseTab), None),
            ("mod+=", None, Some(ShellCommand::ZoomChatIn)),
            ("mod+-", None, Some(ShellCommand::ZoomChatOut)),
            ("mod+0", None, Some(ShellCommand::ZoomChatReset)),
            ("mod+shift+p", None, None),
            ("mod+,", None, None),
            ("mod+s", None, None),
            ("t", None, None),
        ];
        let keymap = Keymap::defaults();
        for &(chord, instead, beside) in CASES {
            let pressed = stroke(chord);
            assert_eq!(
                (keymap.resolve(&pressed), keymap.alongside(&pressed)),
                (instead, beside),
                "{chord}"
            );
        }
    }

    #[test]
    fn overrides_move_unbind_and_free_the_default_that_held_the_chord() {
        const CASES: &[(Override, &str, Runs)] = &[
            (
                ("tabs.new", "mod+shift+k"),
                "mod+shift+k",
                Some(ShellCommand::NewTab),
            ),
            (("tabs.new", "mod+shift+k"), "mod+t", None),
            (("tabs.new", ""), "mod+t", None),
            (("editor.save", "mod+s"), "mod+s", None),
            (
                ("agent.newChat", "mod+t"),
                "mod+t",
                Some(ShellCommand::NewChat),
            ),
        ];
        for &(chosen, typed, expected) in CASES {
            let keymap = Keymap::with_overrides([chosen]);
            assert_eq!(keymap.resolve(&stroke(typed)), expected, "{chosen:?}");
        }
    }

    #[test]
    fn forwarded_bindings_cover_the_bound_shell_commands_and_the_project_slots() {
        let keymap = Keymap::with_overrides([("tabs.new", "")]);
        let bindings = keymap.forwarded(&[]);
        let commands: Vec<&str> = bindings.iter().map(|b| b.command().as_str()).collect();
        assert!(commands.contains(&"tabs.closeActive"));
        assert!(commands.contains(&"agent.zoomIn"));
        assert!(!commands.contains(&"tabs.new"));
        assert!(commands.contains(&"navigation.selectProject.1"));
        assert!(!commands.contains(&"files.search"), "no surface shows");
        let live = keymap.forwarded(&[ContextualCommand::SearchFiles]);
        let search = live.iter().find(|b| b.command().as_str() == "files.search");
        assert_eq!(
            search.map(KeyBinding::precedence),
            Some(protocol::Precedence::Page),
            "a showing surface forwards its chord page-first"
        );
        assert!(bindings.iter().all(|b| !b.keys().as_str().contains(' ')));
    }
}
