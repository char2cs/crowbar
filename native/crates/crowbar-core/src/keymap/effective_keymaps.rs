//! Effective keymap resolution — the algorithm "keymap resolution" actually
//! names.
//!
//! Ported from `web/src/features/keymaps/utils/effective-keymaps.ts`.
//! **No TS test file exists for this module** — `tier-a-denominator.md` §3's
//! own finding: *"the file that most literally IS 'keymap resolution' has
//! zero dedicated tests… new tests would have to be authored, not
//! translated."* Every test in this module is authored against the TS
//! source's own semantics (read directly, branch by branch — see below),
//! not translated from an existing suite. Three of the trickiest are
//! mutation-tested; see their doc comments for the real `cargo test` failure
//! output.
//!
//! The effective binding for each command is computed by merging, in order
//! of **increasing** precedence:
//!   1. command default chord (from the registry)
//!   2. active preset overrides
//!   3. user overrides
//!
//! All chords are normalized ([`super::chord::normalize_chord`]) so conflict
//! detection compares canonical forms — `"Shift+Mod+T"` and `"mod+shift+t"`
//! are the same chord for this purpose.
//!
//! # A branch worth stating rather than silently reproducing
//!
//! TS `resolveBinding`'s override check is `if (userChord != null)` — an
//! override whose value is the **empty string** is not `null`/`undefined`,
//! so it still takes the `'user'` branch, and normalizes to `''`
//! (`normalizeChord('')`, since `parseChord('')` returns `null`). Net effect:
//! a command can have `source: 'user', chord: ''`, which reads as "the user
//! explicitly unbound this command" — a real, distinct outcome from "no
//! override exists," even though the two inputs (missing key vs. an
//! empty-string value) look similar. [`resolve_binding`] keeps this exactly:
//! `opts.overrides.get(command.id)` returning `Some("")` still takes the
//! `User` branch. See `an_empty_string_override_still_wins_as_user_source`
//! below, and its mutation proof.

use std::collections::HashMap;

use super::chord::normalize_chord;
use super::presets::get_preset;
use super::registry::COMMANDS;
use super::types::{BindingSource, Command, EffectiveBinding, KeymapOverrides, KeymapPresetId};

/// Mirrors TS `ResolveOptions`. `commands: None` means "the full registry"
/// (TS `commands?: Command[]` defaulting to `COMMANDS`) — tests pass
/// `Some(&[...])` to exercise the algorithm against a small, hand-built set
/// without the full registry's noise.
pub struct ResolveOptions<'a> {
    pub preset: KeymapPresetId,
    pub overrides: &'a KeymapOverrides,
    pub commands: Option<&'a [Command]>,
}

/// Mirrors `resolveBinding`: resolve the effective binding for a single
/// command.
#[must_use]
pub fn resolve_binding(command: &Command, opts: &ResolveOptions<'_>) -> EffectiveBinding {
    if let Some(user_chord) = opts.overrides.get(command.id) {
        return EffectiveBinding {
            command_id: command.id.to_string(),
            chord: normalize_chord(user_chord),
            source: BindingSource::User,
        };
    }
    if let Some(preset_chord) = get_preset(opts.preset).binding_for(command.id) {
        return EffectiveBinding {
            command_id: command.id.to_string(),
            chord: normalize_chord(preset_chord),
            source: BindingSource::Preset,
        };
    }
    EffectiveBinding {
        command_id: command.id.to_string(),
        chord: normalize_chord(command.default_chord),
        source: BindingSource::Default,
    }
}

/// Mirrors `getEffectiveBindings`: resolve every command into an effective
/// binding. An override keyed on a command id absent from the command set
/// (registry unknown, or excluded by a caller-supplied `commands` slice)
/// never surfaces here — this only ever iterates the command set, never the
/// overrides map's own keys.
#[must_use]
pub fn get_effective_bindings(opts: &ResolveOptions<'_>) -> Vec<EffectiveBinding> {
    let commands = opts.commands.unwrap_or(&COMMANDS);
    commands
        .iter()
        .map(|command| resolve_binding(command, opts))
        .collect()
}

/// Mirrors `getEffectiveChordMap`: map of commandId -> effective chord.
#[must_use]
pub fn get_effective_chord_map(opts: &ResolveOptions<'_>) -> HashMap<String, String> {
    get_effective_bindings(opts)
        .into_iter()
        .map(|binding| (binding.command_id, binding.chord))
        .collect()
}

/// Mirrors `findConflictingCommands`: command ids that would conflict if
/// `chord` were assigned to `command_id`, given the current options
/// (excluding `command_id` itself).
#[must_use]
pub fn find_conflicting_commands(
    command_id: &str,
    chord: &str,
    opts: &ResolveOptions<'_>,
) -> Vec<String> {
    let normalized = normalize_chord(chord);
    if normalized.is_empty() {
        return Vec::new();
    }
    get_effective_bindings(opts)
        .into_iter()
        .filter(|binding| binding.command_id != command_id && binding.chord == normalized)
        .map(|binding| binding.command_id)
        .collect()
}

#[cfg(test)]
mod tests {
    use super::{
        BindingSource, ResolveOptions, find_conflicting_commands, get_effective_bindings,
        get_effective_chord_map, resolve_binding,
    };
    use crate::keymap::registry::{COMMANDS, PANE_SPLIT_DOWN, PANE_SPLIT_RIGHT, TAB_REOPEN_CLOSED};
    use crate::keymap::types::{Command, CommandCategory, KeymapOverrides, KeymapPresetId};

    // A small, hand-built command set — every test below that doesn't need
    // the full registry uses this instead, so a failure points at the
    // algorithm rather than at registry data incidentally changing under it.
    const ALPHA: Command = Command {
        id: "test.alpha",
        label: "Alpha",
        category: CommandCategory::Panes,
        default_chord: "mod+a",
        live_editable: true,
    };
    const BETA: Command = Command {
        id: "test.beta",
        label: "Beta",
        category: CommandCategory::Panes,
        default_chord: "mod+b",
        live_editable: true,
    };
    const TEST_COMMANDS: [Command; 2] = [ALPHA, BETA];

    fn overrides(pairs: &[(&str, &str)]) -> KeymapOverrides {
        pairs
            .iter()
            .map(|(k, v)| ((*k).to_string(), (*v).to_string()))
            .collect()
    }

    fn opts(preset: KeymapPresetId, overrides: &KeymapOverrides) -> ResolveOptions<'_> {
        ResolveOptions {
            preset,
            overrides,
            commands: Some(&TEST_COMMANDS),
        }
    }

    // --- precedence: default < preset < user ---

    /// # Mutation-tested: override-over-preset precedence
    ///
    /// Swapped `resolve_binding`'s two `if let` blocks so the preset lookup
    /// ran before the override lookup (checked preset first, returned early
    /// on a preset hit, only fell through to the override check for
    /// commands the active preset doesn't touch), then ran
    /// `cargo test -p crowbar-core --lib keymap::effective_keymaps`. Real
    /// output:
    ///
    /// ```text
    /// thread 'keymap::effective_keymaps::tests::a_user_override_wins_over_a_preset_binding_for_the_same_command' panicked at crates/crowbar-core/src/keymap/effective_keymaps.rs:186:9:
    /// assertion `left == right` failed
    ///   left: "mod+]"
    ///  right: "mod+z"
    /// test result: FAILED. 12 passed; 1 failed; 0 ignored; 0 measured; 157 filtered out; finished in 0.00s
    /// ```
    ///
    /// Only this test failed — every other test in the module, including
    /// `preset_binding_wins_over_the_command_default_when_no_override_exists`
    /// right below it, still passed, confirming this is the one actually
    /// pinning override-beats-preset (not just "a preset binding exists at
    /// all"). Reverted immediately after.
    #[test]
    fn a_user_override_wins_over_a_preset_binding_for_the_same_command() {
        // PANE_SPLIT_RIGHT is remapped by the real Compact preset (to
        // "mod+]"); giving it a user override too proves the override wins
        // over a REAL preset entry, not just a synthetic one.
        let ov = overrides(&[(PANE_SPLIT_RIGHT, "mod+z")]);
        let o = ResolveOptions {
            preset: KeymapPresetId::Compact,
            overrides: &ov,
            commands: None,
        };
        let split_right = COMMANDS.iter().find(|c| c.id == PANE_SPLIT_RIGHT).unwrap();
        let binding = resolve_binding(split_right, &o);
        assert_eq!(binding.chord, "mod+z");
        assert_eq!(binding.source, BindingSource::User);
    }

    #[test]
    fn preset_binding_wins_over_the_command_default_when_no_override_exists() {
        let ov = overrides(&[]);
        let o = ResolveOptions {
            preset: KeymapPresetId::Compact,
            overrides: &ov,
            commands: None,
        };
        let split_right = COMMANDS.iter().find(|c| c.id == PANE_SPLIT_RIGHT).unwrap();
        let binding = resolve_binding(split_right, &o);
        assert_eq!(binding.chord, "mod+]");
        assert_eq!(binding.source, BindingSource::Preset);
    }

    #[test]
    fn falls_back_to_the_command_default_when_neither_override_nor_preset_has_a_binding() {
        let ov = overrides(&[]);
        let o = opts(KeymapPresetId::Default, &ov);
        let binding = resolve_binding(&ALPHA, &o);
        assert_eq!(binding.chord, "mod+a");
        assert_eq!(binding.source, BindingSource::Default);
    }

    // --- the empty-string-override edge case ---

    /// # Mutation-tested: an empty override still counts as a user binding
    ///
    /// Changed the override lookup from `opts.overrides.get(command.id)` to
    /// `opts.overrides.get(command.id).filter(|c| !c.is_empty())`,
    /// simulating the plausible-but-wrong assumption that an empty override
    /// means "no override," then ran
    /// `cargo test -p crowbar-core --lib keymap::effective_keymaps`. Real
    /// output:
    ///
    /// ```text
    /// thread 'keymap::effective_keymaps::tests::an_empty_string_override_still_wins_as_user_source' panicked at crates/crowbar-core/src/keymap/effective_keymaps.rs:224:9:
    /// assertion `left == right` failed
    ///   left: "mod+a"
    ///  right: ""
    /// test result: FAILED. 12 passed; 1 failed; 0 ignored; 0 measured; 157 filtered out; finished in 0.00s
    /// ```
    ///
    /// Only this test failed. Reverted immediately after.
    #[test]
    fn an_empty_string_override_still_wins_as_user_source() {
        let ov = overrides(&[(ALPHA.id, "")]);
        let o = opts(KeymapPresetId::Default, &ov);
        let binding = resolve_binding(&ALPHA, &o);
        assert_eq!(binding.chord, "");
        assert_eq!(binding.source, BindingSource::User);
    }

    // --- an override naming an unknown command id ---

    #[test]
    fn an_override_naming_a_command_id_outside_the_resolved_set_is_silently_ignored() {
        // get_effective_bindings iterates the COMMAND SET, never the
        // overrides map's own keys, so an override for an id that isn't in
        // `commands` never surfaces anywhere in the output — not applied,
        // not reported, not an error.
        let ov = overrides(&[("no.such.command", "mod+z")]);
        let o = opts(KeymapPresetId::Default, &ov);
        let bindings = get_effective_bindings(&o);
        assert_eq!(bindings.len(), TEST_COMMANDS.len());
        assert!(bindings.iter().all(|b| b.command_id != "no.such.command"));
        // ALPHA and BETA are both unaffected — the stray override changed
        // nothing about the commands that DO exist.
        assert_eq!(
            bindings
                .iter()
                .find(|b| b.command_id == ALPHA.id)
                .unwrap()
                .source,
            BindingSource::Default
        );
    }

    // --- conflict detection ---

    #[test]
    fn two_commands_resolving_to_the_same_chord_conflict_with_each_other() {
        let ov = overrides(&[(BETA.id, "mod+a")]); // BETA now collides with ALPHA's default
        let o = opts(KeymapPresetId::Default, &ov);
        let conflicts = find_conflicting_commands(ALPHA.id, "mod+a", &o);
        assert_eq!(conflicts, vec![BETA.id.to_string()]);
    }

    /// # Mutation-tested: a command never conflicts with its own binding
    ///
    /// Removed the `binding.command_id != command_id` self-exclusion clause
    /// from `find_conflicting_commands`'s filter, then ran
    /// `cargo test -p crowbar-core --lib keymap::effective_keymaps`. Real
    /// output:
    ///
    /// ```text
    /// ---- keymap::effective_keymaps::tests::find_conflicting_commands_excludes_the_queried_command_even_when_its_own_chord_matches stdout ----
    /// thread '...' panicked at crates/crowbar-core/src/keymap/effective_keymaps.rs:271:9:
    /// assertion `left == right` failed
    ///   left: ["test.alpha"]
    ///  right: []
    ///
    /// ---- keymap::effective_keymaps::tests::two_commands_resolving_to_the_same_chord_conflict_with_each_other stdout ----
    /// thread '...' panicked at crates/crowbar-core/src/keymap/effective_keymaps.rs:256:9:
    /// assertion `left == right` failed
    ///   left: ["test.alpha", "test.beta"]
    ///  right: ["test.beta"]
    ///
    /// test result: FAILED. 11 passed; 2 failed; 0 ignored; 0 measured; 157 filtered out; finished in 0.00s
    /// ```
    ///
    /// **Two** tests failed, not one — worth stating rather than trimming to
    /// fit a tidier "only the targeted test failed" narrative.
    /// `two_commands_resolving_to_the_same_chord_conflict_with_each_other`
    /// (above) queries `find_conflicting_commands(ALPHA.id, "mod+a", ...)`,
    /// and `"mod+a"` also happens to be ALPHA's own default chord — so
    /// removing self-exclusion made ALPHA spuriously report a conflict with
    /// itself there too, incidentally, on top of the test purpose-built to
    /// catch exactly this. Both failures are real evidence the guard does
    /// something; every other test (both presets, precedence, the
    /// unknown-command-id case, the empty-chord early return) still passed
    /// unmutated. Reverted immediately after.
    #[test]
    fn find_conflicting_commands_excludes_the_queried_command_even_when_its_own_chord_matches() {
        let ov = overrides(&[]);
        let o = opts(KeymapPresetId::Default, &ov);
        // ALPHA's own effective chord is "mod+a" (its default). Asking
        // "what would conflict if ALPHA were assigned mod+a" must not
        // report ALPHA against itself.
        let conflicts = find_conflicting_commands(ALPHA.id, "mod+a", &o);
        assert_eq!(conflicts, Vec::<String>::new());
    }

    #[test]
    fn find_conflicting_commands_returns_empty_for_an_unparseable_chord() {
        let ov = overrides(&[]);
        let o = opts(KeymapPresetId::Default, &ov);
        assert_eq!(
            find_conflicting_commands(ALPHA.id, "", &o),
            Vec::<String>::new()
        );
        assert_eq!(
            find_conflicting_commands(ALPHA.id, "shift+alt", &o),
            Vec::<String>::new()
        );
    }

    #[test]
    fn find_conflicting_commands_normalizes_the_queried_chord_before_comparing() {
        // BETA's default is "mod+b". Querying with a differently-cased,
        // differently-ordered spelling of the same chord must still match.
        let ov = overrides(&[]);
        let o = opts(KeymapPresetId::Default, &ov);
        let conflicts = find_conflicting_commands(ALPHA.id, "B+Mod", &o);
        assert_eq!(conflicts, vec![BETA.id.to_string()]);
    }

    #[test]
    fn find_conflicting_commands_can_report_more_than_one_colliding_command() {
        let ov = overrides(&[(ALPHA.id, "mod+z"), (BETA.id, "mod+z")]);
        let o = opts(KeymapPresetId::Default, &ov);
        // Neither ALPHA nor BETA is the query command, so both come back.
        let conflicts = find_conflicting_commands("test.gamma", "mod+z", &o);
        let mut sorted = conflicts;
        sorted.sort();
        assert_eq!(sorted, vec![ALPHA.id.to_string(), BETA.id.to_string()]);
    }

    // --- get_effective_chord_map ---

    #[test]
    fn effective_chord_map_has_one_entry_per_command_in_the_resolved_set() {
        let ov = overrides(&[]);
        let o = opts(KeymapPresetId::Default, &ov);
        let map = get_effective_chord_map(&o);
        assert_eq!(map.len(), TEST_COMMANDS.len());
        assert_eq!(map.get(ALPHA.id), Some(&"mod+a".to_string()));
        assert_eq!(map.get(BETA.id), Some(&"mod+b".to_string()));
    }

    // --- integration against the real 19-command registry ---

    #[test]
    fn every_registry_command_resolves_to_its_own_normalized_default_under_the_default_preset() {
        let ov = overrides(&[]);
        let o = ResolveOptions {
            preset: KeymapPresetId::Default,
            overrides: &ov,
            commands: None,
        };
        let map = get_effective_chord_map(&o);
        for command in &COMMANDS {
            assert_eq!(
                map.get(command.id).map(String::as_str),
                Some(command.default_chord),
                "command {} did not resolve to its own default",
                command.id
            );
        }
    }

    #[test]
    fn switching_to_compact_re_resolves_exactly_the_three_remapped_commands() {
        let ov = overrides(&[]);
        let o = ResolveOptions {
            preset: KeymapPresetId::Compact,
            overrides: &ov,
            commands: None,
        };
        let map = get_effective_chord_map(&o);
        assert_eq!(map.get(PANE_SPLIT_RIGHT).map(String::as_str), Some("mod+]"));
        assert_eq!(
            map.get(PANE_SPLIT_DOWN).map(String::as_str),
            Some("mod+shift+]")
        );
        assert_eq!(
            map.get(TAB_REOPEN_CLOSED).map(String::as_str),
            Some("mod+shift+r")
        );
        // Everything else still resolves to its own default.
        let unaffected = COMMANDS
            .iter()
            .filter(|c| ![PANE_SPLIT_RIGHT, PANE_SPLIT_DOWN, TAB_REOPEN_CLOSED].contains(&c.id));
        for command in unaffected {
            assert_eq!(
                map.get(command.id).map(String::as_str),
                Some(command.default_chord)
            );
        }
    }
}
