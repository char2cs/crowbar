//! Chords of the shortcuts: reading them, capturing a new one and writing the overrides.

use std::collections::BTreeMap;

use super::SettingsState;
use crate::chord_source::ChordSource;
use crate::effect::Effect;
use crate::key::Key;
use crate::keymap::Keymap;
use crate::pressed::Pressed;
use crate::refusal::Refusal;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;
use crate::value::Value;
use chord::Chord;

impl SettingsState {
    /// The chord of `shortcut` and whether the user set it.
    pub fn chord(
        &self,
        group: &ShortcutGroup,
        shortcut: &Shortcut,
    ) -> (Option<Chord>, ChordSource) {
        let keymap = Keymap::new(&self.schema, &self.document);
        (
            keymap.chord(group, shortcut),
            keymap.source(group, shortcut),
        )
    }

    /// Labels of the other commands bound to the same chord as `shortcut`.
    pub fn conflicts(
        &self,
        group: &ShortcutGroup,
        shortcut: &Shortcut,
    ) -> Vec<String> {
        Keymap::new(&self.schema, &self.document).conflicts(group, shortcut)
    }

    /// Whether any shortcut has a custom chord.
    pub fn has_custom_chords(&self) -> bool {
        Keymap::new(&self.schema, &self.document).has_custom()
    }

    /// The label of `chord` on this platform.
    pub fn label(
        &self,
        chord: &Chord,
    ) -> String {
        chord.label(self.primary)
    }

    /// Starts (or, on the capturing button, stops) listening for a chord for `command`.
    pub fn toggle_capture(
        &mut self,
        command: &str,
    ) {
        self.capture.begin(command);
    }

    /// The command whose chord is being captured.
    pub fn capturing(&self) -> Option<&str> {
        self.capture.target()
    }

    /// Why the last key press was turned down.
    pub fn capture_refusal(&self) -> Option<Refusal> {
        self.capture.refusal()
    }

    /// Feeds a key press to the capture; returns the write when it completed a chord.
    pub fn press_key(
        &mut self,
        pressed: &Pressed,
    ) -> Option<Effect> {
        let (command, chord) = self.capture.feed(pressed, self.primary)?;
        self.set_chord(&command, Some(chord.as_str()))
    }

    /// Drops the custom chord of `command`, which restores its default.
    pub fn clear_chord(
        &mut self,
        command: &str,
    ) -> Option<Effect> {
        self.set_chord(command, None)
    }

    fn set_chord(
        &mut self,
        command: &str,
        chord: Option<&str>,
    ) -> Option<Effect> {
        let (group, shortcut) = self
            .schema
            .shortcuts()
            .find(|(_, s)| s.command == command)?;
        let mut map = Keymap::new(&self.schema, &self.document).overrides(group);
        let unchanged_default =
            chord.is_some_and(|text| shortcut.default.as_ref().map(Chord::as_str) == Some(text));
        match chord {
            Some(text) if !unchanged_default => map.insert(command.to_owned(), text.to_owned()),
            _ => map.remove(command),
        };
        let key = group.overrides.clone();
        Some(self.write(&key, Value::Map(map)))
    }

    pub(super) fn clear_overrides(
        &mut self,
        key: &Key,
    ) -> Option<Effect> {
        let has_any = self
            .document
            .value(key)
            .and_then(Value::as_map)
            .is_some_and(|map| !map.is_empty());
        has_any.then(|| self.write(key, Value::Map(BTreeMap::new())))
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::action::Action;
    use crate::button_spec::ButtonSpec;
    use crate::modifier::Modifier;
    use crate::page::Page;
    use crate::schema::Schema;
    use crate::snapshot::Snapshot;
    use crate::test_support::{Case, run_cases};
    use chord::Primary;

    fn state() -> SettingsState {
        let keys = Page::new("keybindings", "Keybindings", "keyboard").block(
            ShortcutGroup::new("Tabs", "overrides")
                .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t"))
                .shortcut(Shortcut::new("tabs.close", "Close active tab", "mod+w")),
        );
        let schema = Schema::new(vec![keys]).unwrap();
        SettingsState::new(schema, Snapshot::default(), None, Primary::Control)
    }

    fn write(
        key: &str,
        value: impl Into<Value>,
    ) -> Effect {
        Effect::Write(Key::from(key), value.into())
    }

    fn pressed(
        key: &str,
        held: &[Modifier],
    ) -> Pressed {
        Pressed::new(key, held)
    }

    fn overrides(pairs: &[(&str, &str)]) -> Effect {
        let map = pairs
            .iter()
            .map(|&(command, chord)| (command.to_owned(), chord.to_owned()))
            .collect();
        write("overrides", Value::Map(map))
    }

    fn group_of(state: &SettingsState) -> (ShortcutGroup, Shortcut) {
        let (group, shortcut) = state.schema().shortcuts().next().unwrap();
        (group.clone(), shortcut.clone())
    }

    fn capture_stores_a_chord_and_refuses_bare_keys() {
        let mut state = state();
        state.toggle_capture("tabs.new");
        assert_eq!(state.capturing(), Some("tabs.new"));
        assert_eq!(state.press_key(&pressed("a", &[])), None);
        assert_eq!(state.capture_refusal(), Some(Refusal::BareKey));
        assert_eq!(
            state.press_key(&pressed("n", &[Modifier::Control, Modifier::Shift])),
            Some(overrides(&[("tabs.new", "mod+shift+n")]))
        );
        assert_eq!(state.capturing(), None);
        let (group, shortcut) = group_of(&state);
        let (chord, source) = state.chord(&group, &shortcut);
        assert_eq!(
            chord.map(|chord| state.label(&chord)),
            Some("Ctrl+Shift+N".to_owned())
        );
        assert_eq!(source, ChordSource::Custom);
        assert!(state.has_custom_chords());
    }

    fn escape_leaves_capture_and_the_default_chord_is_not_stored_as_custom() {
        let mut state = state();
        state.toggle_capture("tabs.new");
        assert_eq!(state.press_key(&pressed("escape", &[])), None);
        assert_eq!(state.capturing(), None);
        state.toggle_capture("tabs.new");
        assert_eq!(
            state.press_key(&pressed("t", &[Modifier::Control])),
            Some(overrides(&[])),
            "the default chord clears the override"
        );
        assert!(!state.has_custom_chords());
    }

    fn conflicts_clear_and_reset_all() {
        let mut state = state();
        state.toggle_capture("tabs.close");
        state.press_key(&pressed("t", &[Modifier::Control, Modifier::Shift]));
        state.toggle_capture("tabs.new");
        state.press_key(&pressed("t", &[Modifier::Control, Modifier::Shift]));
        let (group, shortcut) = group_of(&state);
        assert_eq!(
            state.conflicts(&group, &shortcut),
            ["Close active tab"],
            "the same chord is flagged"
        );
        assert_eq!(
            state.clear_chord("tabs.new"),
            Some(overrides(&[("tabs.close", "mod+shift+t")]))
        );
        let reset = ButtonSpec::new("Reset all", Action::ResetOverrides(Key::from("overrides")));
        assert_eq!(state.press_button(&reset), Some(overrides(&[])));
        assert_eq!(
            state.press_button(&reset),
            None,
            "nothing custom is left to reset"
        );
        assert_eq!(state.clear_chord("unknown.command"), None);
    }

    const CASES: &[Case] = &[
        (
            "capture stores a chord and refuses bare keys",
            capture_stores_a_chord_and_refuses_bare_keys,
        ),
        (
            "escape leaves capture and the default chord is not stored as custom",
            escape_leaves_capture_and_the_default_chord_is_not_stored_as_custom,
        ),
        (
            "conflicts clear and reset all",
            conflicts_clear_and_reset_all,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
