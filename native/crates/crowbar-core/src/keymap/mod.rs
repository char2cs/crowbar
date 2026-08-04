//! Keymap resolution — spec §4.2's `"keymap resolution"` bucket of
//! `crowbar-core`, and the example item §11.2 itself cites:
//! *"`crowbar_core::keymap::resolve` passes the ported test suite at ≥98%
//! coverage."*
//!
//! Ported from all five files `native/mapping/tier-a-denominator.md` §3
//! ("Keymap resolution") classifies as LIVE and portable (of
//! `web/src/features/keymaps/`'s 733 total lines):
//!
//! | module | ported from | TS tests? |
//! |---|---|---|
//! | [`types`] | `types.ts` | none — authored here |
//! | [`registry`] | `registry.ts` | `registry.test.ts` (9 cases), ported |
//! | [`presets`] | `defaults/keybinding-presets.ts` | none — authored here |
//! | [`chord`] | `utils/chord.ts` (4 of 6 exports) | `chord.test.ts` (7 cases, 5 for ported exports), ported |
//! | [`effective_keymaps`] | `utils/effective-keymaps.ts` | none — authored here |
//!
//! # What this item did not port, and why
//!
//! * **`chord.ts`'s `chordFromEvent`/`eventMatchesChord`** — take a DOM
//!   `KeyboardEvent` directly. See [`chord`]'s module doc for the full
//!   account of why this is a reimplementation-at-the-boundary rather than a
//!   port, and where that responsibility has to live in GPUI instead.
//! * **`chord.ts`'s `MOD_ORDER` export** — zero non-test importers anywhere
//!   in `web/src`; a dead export in a live file, not counted in the
//!   survey's own "4 of 6" figure either. See [`chord`]'s module doc.
//! * **`stores/store.ts`** (100 lines) — the zustand store persisting the
//!   active preset + user overrides to `localStorage`. Phase 4
//!   (`Entity<T>`) + D6 (persistence mechanism deleted, replaced by the
//!   daemon's `/v0/settings/ui`). [`types::KeymapOverrides`] is the shape
//!   that store would hold; nothing here constructs or persists one.
//! * **The four keyboard hooks** (`use-effective-keymap`,
//!   `use-save-keyboard`, `use-sidebar-tab-keyboard`,
//!   `use-workspace-switcher-keyboard`) and the dead
//!   `use-command-shortcut.ts` stub — `useEffect` + DOM
//!   `addEventListener('keydown', …)` wiring. Not logic: GPUI has its own
//!   native action/keybinding dispatch system (see the gpui skill), so this
//!   glue is replaced, not translated, when the port reaches Phase 3.
//!
//! See `native/mapping/core-keymap.md` for the full account, including the
//! two non-portable chord exports and where their responsibility has to
//! land in the eventual GPUI integration.

pub mod chord;
pub mod effective_keymaps;
pub mod presets;
pub mod registry;
pub mod types;

pub use effective_keymaps::{
    ResolveOptions, find_conflicting_commands, get_effective_bindings, get_effective_chord_map,
    resolve_binding,
};
pub use presets::{
    COMPACT_PRESET, DEFAULT_PRESET, KEYMAP_PRESET_OPTIONS, KEYMAP_PRESETS, get_preset,
    parse_keymap_preset_id,
};
pub use registry::{CATEGORY_ORDER, COMMANDS, get_command};
pub use types::{
    BindingSource, Command, CommandCategory, EffectiveBinding, KeymapOverrides, KeymapPreset,
    KeymapPresetId,
};
