//! Keymap domain types.
//!
//! Ported from `web/src/features/keymaps/types.ts`. A "chord" is a single
//! keystroke combination (modifiers + a key), stored in a normalized,
//! platform-neutral string form such as `mod+\`, `mod+shift+t`,
//! `mod+alt+arrowleft`. `mod` resolves to Cmd on macOS and Ctrl elsewhere —
//! see [`super::chord`] for the grammar itself.
//!
//! # Shape differences from the TS source, and why
//!
//! * **`CommandCategory` and `KeymapPresetId` are enums, not strings.** The TS
//!   source declares them as closed string unions (`'Panes' | 'Tabs' | ...`,
//!   `'default' | 'compact'`). Both sets are genuinely closed — the TS side
//!   itself defends this at runtime with `isKeymapPresetId`, a type guard
//!   that exists only because a `string` can hold values the union forbids.
//!   A Rust enum makes the invalid states this guard exists to catch
//!   unrepresentable at the type level; see [`super::presets::parse_keymap_preset_id`]
//!   for where that guard's behaviour is preserved anyway, for the one call
//!   site (persisted-preference deserialisation, `stores/store.ts`, Phase 4 —
//!   not ported here) that still needs to validate an untrusted string.
//! * **`KeymapOverrides` drops the `Record<string, string>` alias in favour of
//!   a plain `HashMap<String, String>`.** The TS doc comment on the type says
//!   the value is "(or null to mean unbound)" but the type itself never
//!   allows `null` — see [`super::effective_keymaps`]'s module doc for why
//!   that comment describes a distinction the code never actually makes: a
//!   missing key and a `null`/`undefined` value are handled identically by
//!   `resolveBinding`'s own `!= null` check, so there is nothing for an
//!   `Option`-valued map to represent that "absent" doesn't already cover.

use std::collections::HashMap;

/// Categories used to group commands in the Keybindings settings tab.
///
/// Mirrors the TS string union `'Panes' | 'Tabs' | 'Editor' | 'Navigation' |
/// 'Chats'`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CommandCategory {
    Panes,
    Tabs,
    Editor,
    Navigation,
    Chats,
}

/// A command is a finite, knowable action that can be bound to a key chord.
/// Commands are extracted from the keyboard hooks that already exist in the
/// codebase — this is not an open-ended command system.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Command {
    /// Stable identifier, e.g. `panes.splitRight`.
    pub id: &'static str,
    /// Human-readable label shown in the settings tab.
    pub label: &'static str,
    pub category: CommandCategory,
    /// Default chord in normalized string form.
    pub default_chord: &'static str,
    /// Whether the live binding is resolved from the registry (live-editable)
    /// or still hardcoded in a hook and only represented here for display.
    pub live_editable: bool,
}

/// The set of selectable presets. Only some are "real" (populated).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum KeymapPresetId {
    Default,
    Compact,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct KeymapPreset {
    pub id: KeymapPresetId,
    pub label: &'static str,
    /// Whether this preset ships a real remap set.
    pub populated: bool,
    /// commandId -> chord overrides applied on top of command defaults, as
    /// `(command_id, chord)` pairs rather than a `HashMap` — see
    /// [`KeymapPreset::binding_for`]. Both built-in presets have at most 3
    /// entries (`registry.rs`'s own `get_command` makes the same
    /// scan-over-a-tiny-const-table call for the same reason), so a linear
    /// scan costs nothing and needs no runtime map construction to make this
    /// a `const` table.
    pub bindings: &'static [(&'static str, &'static str)],
}

impl KeymapPreset {
    /// This preset's override chord for `command_id`, if it has one.
    #[must_use]
    pub fn binding_for(&self, command_id: &str) -> Option<&'static str> {
        self.bindings
            .iter()
            .find(|(id, _)| *id == command_id)
            .map(|(_, chord)| *chord)
    }
}

/// A user override: commandId -> chord. See the module doc for why this is
/// not `HashMap<String, Option<String>>` despite the TS type's "or null"
/// comment.
pub type KeymapOverrides = HashMap<String, String>;

/// Where a resolved chord came from, in ascending precedence.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum BindingSource {
    Default,
    Preset,
    User,
}

/// One resolved binding plus where it came from.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct EffectiveBinding {
    pub command_id: String,
    pub chord: String,
    pub source: BindingSource,
}
