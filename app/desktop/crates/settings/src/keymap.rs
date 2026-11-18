//! Effective chords of the shortcuts: defaults overridden by what the user set.

use std::collections::BTreeMap;

use crate::chord_source::ChordSource;
use crate::document::Document;
use crate::page::Page;
use crate::schema::Schema;
use crate::shortcut::Shortcut;
use crate::shortcut_group::ShortcutGroup;
use chord::Chord;

#[derive(Debug, Clone, Copy)]
pub(crate) struct Keymap<'a> {
    schema: &'a Schema,
    document: &'a Document,
}

impl<'a> Keymap<'a> {
    pub(crate) fn new(
        schema: &'a Schema,
        document: &'a Document,
    ) -> Self {
        Self { schema, document }
    }

    pub(crate) fn overrides(
        &self,
        group: &ShortcutGroup,
    ) -> BTreeMap<String, String> {
        self.document
            .value(&group.overrides)
            .and_then(|value| value.as_map())
            .cloned()
            .unwrap_or_default()
    }

    pub(crate) fn chord(
        &self,
        group: &ShortcutGroup,
        shortcut: &Shortcut,
    ) -> Option<Chord> {
        match self.overrides(group).get(&shortcut.command) {
            Some(text) => Chord::parse(text),
            None => self
                .preset_chord(group, shortcut)
                .or_else(|| shortcut.default.clone()),
        }
    }

    fn preset_chord(
        &self,
        group: &ShortcutGroup,
        shortcut: &Shortcut,
    ) -> Option<Chord> {
        let active = self.document.value(group.preset.as_ref()?)?;
        let active = active.as_text()?;
        shortcut
            .presets
            .iter()
            .find(|(preset, _)| preset == active)
            .map(|(_, chord)| chord.clone())
    }

    pub(crate) fn source(
        &self,
        group: &ShortcutGroup,
        shortcut: &Shortcut,
    ) -> ChordSource {
        if self.overrides(group).contains_key(&shortcut.command) {
            return ChordSource::Custom;
        }
        if self.preset_chord(group, shortcut).is_some() {
            return ChordSource::Preset;
        }
        ChordSource::Default
    }

    pub(crate) fn conflicts(
        &self,
        group: &ShortcutGroup,
        shortcut: &Shortcut,
    ) -> Vec<String> {
        let Some(chord) = self.chord(group, shortcut) else {
            return Vec::new();
        };
        self.schema
            .shortcuts()
            .filter(|(_, other)| other.command != shortcut.command)
            .filter(|(other_group, other)| self.chord(other_group, other).as_ref() == Some(&chord))
            .map(|(_, other)| other.label.clone())
            .collect()
    }

    pub(crate) fn has_custom(&self) -> bool {
        self.schema
            .pages()
            .iter()
            .flat_map(Page::shortcut_groups)
            .any(|group| !self.overrides(group).is_empty())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::page::Page;
    use crate::test_support::{Case, run_cases};
    use crate::value::Value;
    use crate::values::Values;

    fn schema() -> Schema {
        let group = ShortcutGroup::new("Tabs", "overrides")
            .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t"))
            .shortcut(Shortcut::new(
                "tabs.newTerminal",
                "New terminal tab",
                "mod+j",
            ))
            .shortcut(Shortcut::new(
                "agent.cycleProvider",
                "Cycle chat provider",
                "",
            ));
        Schema::new(vec![
            Page::new("keybindings", "Keybindings", "keyboard").block(group),
        ])
        .unwrap()
    }

    fn document(overrides: &[(&str, &str)]) -> Document {
        let map = overrides
            .iter()
            .map(|&(k, v)| (k.to_owned(), v.to_owned()))
            .collect();
        Document::new(
            Values::default().with("overrides", Value::Map(map)),
            Values::default(),
        )
    }

    fn tabs_new_override(custom: Option<&str>) -> BTreeMap<String, String> {
        let mut own = BTreeMap::new();
        if let Some(text) = custom {
            own.insert("tabs.new".to_owned(), text.to_owned());
        }
        own
    }

    fn first_group(schema: &Schema) -> &ShortcutGroup {
        schema
            .pages()
            .first()
            .and_then(|page| page.shortcut_groups().next())
            .unwrap()
    }

    fn chord_of(
        schema: &Schema,
        document: &Document,
        command: &str,
    ) -> Option<String> {
        let group = first_group(schema);
        let shortcut = group
            .shortcuts
            .iter()
            .find(|s| s.command == command)
            .unwrap();
        Keymap::new(schema, document)
            .chord(group, shortcut)
            .map(|c| c.as_str().to_owned())
    }

    fn overrides_win_over_defaults_and_empty_means_unbound() {
        let schema = schema();
        let plain = document(&[]);
        assert_eq!(
            chord_of(&schema, &plain, "tabs.new").as_deref(),
            Some("mod+t")
        );
        assert_eq!(chord_of(&schema, &plain, "agent.cycleProvider"), None);
        let custom = document(&[("tabs.new", "Mod+Shift+N"), ("tabs.newTerminal", "")]);
        assert_eq!(
            chord_of(&schema, &custom, "tabs.new").as_deref(),
            Some("mod+shift+n")
        );
        assert_eq!(chord_of(&schema, &custom, "tabs.newTerminal"), None);
    }

    fn sources_and_custom_detection() {
        let schema = schema();
        let group = first_group(&schema);
        let custom = document(&[("tabs.new", "mod+n")]);
        let keymap = Keymap::new(&schema, &custom);
        let sources: Vec<ChordSource> = group
            .shortcuts
            .iter()
            .map(|s| keymap.source(group, s))
            .collect();
        assert_eq!(
            sources,
            [
                ChordSource::Custom,
                ChordSource::Default,
                ChordSource::Default
            ]
        );
        assert!(keymap.has_custom());
        assert!(!Keymap::new(&schema, &document(&[])).has_custom());
    }

    fn a_preset_binds_between_the_override_and_the_default() {
        let group = ShortcutGroup::new("Tabs", "overrides")
            .preset_from("preset")
            .shortcut(Shortcut::new("tabs.new", "New tab", "mod+t").in_preset("compact", "mod+r"));
        let pages = vec![Page::new("keybindings", "Keybindings", "keyboard").block(group)];
        let schema = Schema::new(pages).unwrap();
        let group = first_group(&schema);
        let cases = [
            ("default", None, "mod+t", ChordSource::Default),
            ("compact", None, "mod+r", ChordSource::Preset),
            ("compact", Some("mod+k"), "mod+k", ChordSource::Custom),
        ];
        for (preset, custom, chord, source) in cases {
            let own = tabs_new_override(custom);
            let values = Values::default()
                .with("preset", Value::Text(preset.to_owned()))
                .with("overrides", Value::Map(own));
            let document = Document::new(values, Values::default());
            let keymap = Keymap::new(&schema, &document);
            let found = keymap.chord(group, &group.shortcuts[0]);
            assert_eq!(found.as_ref().map(Chord::as_str), Some(chord), "{preset}");
            assert_eq!(
                keymap.source(group, &group.shortcuts[0]),
                source,
                "{preset}"
            );
        }
    }

    fn a_chord_shared_by_two_commands_is_a_conflict_for_both() {
        let schema = schema();
        let group = first_group(&schema);
        let clash = document(&[("tabs.newTerminal", "mod+t")]);
        let keymap = Keymap::new(&schema, &clash);
        let found: Vec<Vec<String>> = group
            .shortcuts
            .iter()
            .map(|s| keymap.conflicts(group, s))
            .collect();
        assert_eq!(found[0], ["New terminal tab"]);
        assert_eq!(found[1], ["New tab"]);
        assert!(
            found[2].is_empty(),
            "an unbound command conflicts with nothing"
        );
    }

    const CASES: &[Case] = &[
        (
            "overrides win over defaults and empty means unbound",
            overrides_win_over_defaults_and_empty_means_unbound,
        ),
        ("sources and custom detection", sources_and_custom_detection),
        (
            "a preset binds between the override and the default",
            a_preset_binds_between_the_override_and_the_default,
        ),
        (
            "a chord shared by two commands is a conflict for both",
            a_chord_shared_by_two_commands_is_a_conflict_for_both,
        ),
    ];

    #[test]
    fn every_scenario_holds() {
        run_cases(CASES);
    }
}
