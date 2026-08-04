//! Keybinding presets.
//!
//! Ported from `web/src/features/keymaps/defaults/keybinding-presets.ts`.
//!
//! [`DEFAULT_PRESET`] is the baseline — it adds no overrides, so every
//! command uses its registry default chord.
//!
//! [`COMPACT_PRESET`] is a real, populated alternate preset (proof that
//! switching the preset re-resolves live bindings). It remaps the
//! editor-split commands onto single-modifier chords so they don't collide
//! with `\`.
//!
//! Additional IDE presets (vscode/jetbrains/sublime/xcode/atom/emacs/zed) are
//! intentionally NOT authored here — see SCOPE in the TS settings tab
//! (`features/settings/components/keybindings-settings.tsx`, not ported:
//! Phase 3 presentation).
//!
//! # `getPreset`'s `?? default` fallback does not survive the port
//!
//! The TS source is `KEYMAP_PRESETS[id] ?? KEYMAP_PRESETS.default` — a
//! defensive fallback for an `id` that reached this function without going
//! through [`parse_keymap_preset_id`] first (TS's structural typing does not
//! stop a `string` cast as `KeymapPresetId` from holding an unrecognised
//! value at runtime, e.g. a corrupted `localStorage` entry deserialised
//! straight into the type). [`get_preset`] here takes a [`super::types::KeymapPresetId`]
//! by value: the enum's two variants are the only inputs that type-check, so
//! the match is exhaustive and the fallback branch has no reachable input to
//! guard against. The validation the fallback existed for still happens —
//! just earlier, in [`parse_keymap_preset_id`], the one function on this
//! boundary that actually accepts an untrusted `&str`.

use super::registry::{PANE_SPLIT_DOWN, PANE_SPLIT_RIGHT, TAB_REOPEN_CLOSED};
use super::types::{KeymapPreset, KeymapPresetId};

pub const DEFAULT_PRESET: KeymapPreset = KeymapPreset {
    id: KeymapPresetId::Default,
    label: "Default",
    populated: true,
    bindings: &[],
};

pub const COMPACT_PRESET: KeymapPreset = KeymapPreset {
    id: KeymapPresetId::Compact,
    label: "Compact",
    populated: true,
    bindings: &[
        // Remap splits onto bracket keys, free up backslash.
        (PANE_SPLIT_RIGHT, "mod+]"),
        (PANE_SPLIT_DOWN, "mod+shift+]"),
        // Reopen closed tab on a single modifier.
        (TAB_REOPEN_CLOSED, "mod+shift+r"),
    ],
};

/// All built-in presets, in the order `KEYMAP_PRESET_OPTIONS` shows them.
pub const KEYMAP_PRESETS: [KeymapPreset; 2] = [DEFAULT_PRESET, COMPACT_PRESET];

/// `(id, label)` pairs mirroring TS `KEYMAP_PRESET_OPTIONS`, e.g. for a
/// preset picker.
pub const KEYMAP_PRESET_OPTIONS: [(KeymapPresetId, &str); 2] = [
    (KeymapPresetId::Default, "Default"),
    (KeymapPresetId::Compact, "Compact (alternate)"),
];

/// Mirrors `getPreset`. Total, not partial — see the module doc for why the
/// TS source's fallback branch has no equivalent here.
#[must_use]
pub fn get_preset(id: KeymapPresetId) -> &'static KeymapPreset {
    match id {
        KeymapPresetId::Default => &DEFAULT_PRESET,
        KeymapPresetId::Compact => &COMPACT_PRESET,
    }
}

/// Mirrors `isKeymapPresetId` — the runtime type guard for an untrusted
/// string (e.g. a persisted preference read back from storage).
#[must_use]
pub fn parse_keymap_preset_id(value: &str) -> Option<KeymapPresetId> {
    match value {
        "default" => Some(KeymapPresetId::Default),
        "compact" => Some(KeymapPresetId::Compact),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::{
        COMPACT_PRESET, DEFAULT_PRESET, KEYMAP_PRESET_OPTIONS, get_preset, parse_keymap_preset_id,
    };
    use crate::keymap::registry::{PANE_SPLIT_DOWN, PANE_SPLIT_RIGHT, TAB_REOPEN_CLOSED};
    use crate::keymap::types::KeymapPresetId;

    // No TS test file exists for keybinding-presets.ts (tier-a-denominator.md
    // §3's finding). Every case below is authored against the TS source's
    // own semantics, not translated from an existing suite.

    #[test]
    fn default_preset_adds_no_overrides() {
        assert!(DEFAULT_PRESET.bindings.is_empty());
    }

    #[test]
    fn both_built_in_presets_are_populated_reached_through_get_preset_not_the_const_directly() {
        // Both existing presets are "real" (populated: true) — the TS doc
        // comment's own distinction ("only some are 'real'") has no
        // unpopulated example yet, so this exercises the field through the
        // same accessor a caller would use rather than asserting a literal
        // on the const declaration.
        assert!(get_preset(KeymapPresetId::Default).populated);
        assert!(get_preset(KeymapPresetId::Compact).populated);
    }

    #[test]
    fn compact_preset_remaps_exactly_the_three_documented_commands() {
        assert_eq!(COMPACT_PRESET.binding_for(PANE_SPLIT_RIGHT), Some("mod+]"));
        assert_eq!(
            COMPACT_PRESET.binding_for(PANE_SPLIT_DOWN),
            Some("mod+shift+]")
        );
        assert_eq!(
            COMPACT_PRESET.binding_for(TAB_REOPEN_CLOSED),
            Some("mod+shift+r")
        );
        assert_eq!(COMPACT_PRESET.bindings.len(), 3);
    }

    #[test]
    fn compact_preset_has_no_override_for_a_command_it_does_not_remap() {
        assert_eq!(COMPACT_PRESET.binding_for("tabs.new"), None);
    }

    #[test]
    fn get_preset_resolves_each_id_to_the_matching_static_preset() {
        assert_eq!(get_preset(KeymapPresetId::Default).label, "Default");
        assert_eq!(get_preset(KeymapPresetId::Compact).label, "Compact");
    }

    #[test]
    fn preset_options_lists_both_presets_in_display_order() {
        assert_eq!(
            KEYMAP_PRESET_OPTIONS.map(|(id, _)| id),
            [KeymapPresetId::Default, KeymapPresetId::Compact]
        );
        assert_eq!(KEYMAP_PRESET_OPTIONS[1].1, "Compact (alternate)");
    }

    #[test]
    fn parse_keymap_preset_id_accepts_only_the_two_known_ids() {
        assert_eq!(
            parse_keymap_preset_id("default"),
            Some(KeymapPresetId::Default)
        );
        assert_eq!(
            parse_keymap_preset_id("compact"),
            Some(KeymapPresetId::Compact)
        );
        assert_eq!(parse_keymap_preset_id("vscode"), None);
        assert_eq!(parse_keymap_preset_id(""), None);
        // Case-sensitive, matching the TS `===` comparisons exactly — a
        // corrupted-casing localStorage value is treated as invalid, not
        // silently normalized.
        assert_eq!(parse_keymap_preset_id("Default"), None);
    }
}
