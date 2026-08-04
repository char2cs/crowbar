//! Settings schema — spec §4.2's `"settings schema"` bucket of
//! `crowbar-core`.
//!
//! Ported from the eight files
//! `native/mapping/tier-a-denominator.md` §4 ("Settings schema") classifies
//! as **LIVE** (reachable at boot, no dialog/pane/tab selection required —
//! see §4's own Liveness section: `main.tsx` → `initializeSettingsStore` →
//! `settings-bootstrap.ts` calls `default-settings.ts` and
//! `normalizeSettings` unconditionally):
//!
//! | module | ported from |
//! |---|---|
//! | [`types`] | `types/settings.ts` + `types/feature.ts` |
//! | [`typography`] | `config/typography-defaults.ts` |
//! | [`defaults`] | `config/default-settings.ts` |
//! | [`normalization`] | `lib/settings-normalization.ts` |
//! | [`font_family`] | `lib/font-family-resolution.ts` |
//! | [`markdown_font_size`] | `lib/markdown-font-size.ts` |
//! | [`ui_font_size`] | `lib/ui-font-size.ts` |
//! | [`raw_value`] | (new — see its module doc) |
//!
//! # Reconciliation against the survey's own reconstruction (P3.72)
//!
//! §4's headline table gives the settings-schema area a Tier A core figure
//! of **629 lines across 9 files**. The denominator-reconciliation section
//! at the end of the survey ("Denominator reconciliation (P3.71)") breaks
//! that 9-file, 629-line figure down explicitly: **554 LIVE** (`types/
//! settings.ts` 81, `types/feature.ts` 3, `default-settings.ts` 98,
//! `typography-defaults.ts` 25, `settings-normalization.ts` 249,
//! `font-family-resolution.ts` 40, `markdown-font-size.ts` 26,
//! `ui-font-size.ts` 32 — exactly the eight files this item was scoped to)
//! **+ 75 CONDITIONAL** (`settings-import-export.ts`, gated behind the
//! Settings → Developer tab's export/import buttons). This item's own
//! brief scoped it to "the settings-schema **LIVE** subset only," and that
//! subset is exactly these eight files, summing to exactly 554 — checked
//! against the survey before writing any Rust, per this item's own
//! instructions. No ninth LIVE file was missed; nothing in the eight needed
//! dropping. `settings-import-export.ts` is correctly out of scope for this
//! item (CONDITIONAL, not LIVE) and is not ported here.
//!
//! # What this item did not port, and why (all CONDITIONAL or out of scope
//! # per §4)
//!
//! * **`types/search.ts`, `config/search-index.ts`, `lib/settings-row-search.ts`,
//!   `lib/settings-tab-visibility.ts`** — the settings-search feature.
//!   CONDITIONAL (Settings dialog search box) and presentation (387 lines of
//!   static label/keyword copy), per §4's own "What is not settings-schema
//!   logic" section.
//! * **`lib/settings-import-export.ts`, `lib/settings-download.ts`,
//!   `lib/diagnostics-export.ts`, `utils/theme-upload.ts`** — all
//!   CONDITIONAL, gated behind specific Settings dialog tabs/buttons.
//! * **`lib/settings-persistence.ts`** — the `localStorage`-backed shim.
//!   Explicitly out of scope per this item's brief (D6; §9.3's
//!   `/v0/settings/ui` is the daemon-side replacement).
//! * **`lib/settings-effects.ts`, `lib/appearance-bootstrap.ts`** — DOM/CSS
//!   application (theme class toggling, pre-hydration FOUC-prevention
//!   cache). Out of scope per this item's brief; §6.1's sealed `Theme`
//!   struct replaces the mechanism entirely, not just the DOM half of it.
//! * **`lib/settings-bootstrap.ts`** — boot orchestration gluing
//!   persistence + normalization + effects. Phase 4 wiring, not logic; the
//!   one embedded rule it has (derive `theme` from `syncSystemTheme` +
//!   `matchMedia`) is `window`-coupled in the TS source and not ported.
//! * **`store.ts`, `stores/agent-providers-store.ts`, `stores/font-store.ts`,
//!   `stores/types/font.ts`** — zustand stores. Phase 4
//!   (`crowbar-state`'s `Entity<T>`), per this crate's `lib.rs` doc and
//!   `crate::workspace`'s own precedent for why reactive state does not
//!   belong here.
//!
//! # A cross-area dependency this item could not defer — reconciled (P3.75)
//!
//! `settings-normalization.ts` imports `normalizeFileTreeDensity` from
//! `web/src/features/file-explorer/lib/file-tree-density.ts` — File-tree
//! model (§5), a *different* Tier A area, which had not yet been ported to
//! `crowbar-core` as of this item. Since that import is unconditional in the
//! boot-time `normalizeSettings` path, this item could not simply omit it
//! without leaving `normalize_settings` unable to reproduce real boot
//! behaviour, so [`types::FileTreeDensity`] and
//! [`types::normalize_file_tree_density`] started as a narrow,
//! explicitly-flagged local duplicate of that file's 3-variant type +
//! normalizer. **File-tree model has since landed** (`crate::file_tree`,
//! P3.75) and reclaimed this duplicate: both names are now re-exports of
//! [`crate::file_tree::density`]'s definitions, not a second declaration —
//! see [`types`]'s module doc and `crate::file_tree::density`'s module doc
//! for the full reconciliation account.

pub mod defaults;
pub mod font_family;
pub mod markdown_font_size;
pub mod normalization;
pub mod raw_value;
pub mod types;
pub mod typography;
pub mod ui_font_size;

pub use defaults::{DEFAULT_THEME_ID, default_settings};
pub use font_family::{
    get_primary_font_family, normalize_configured_font_family, resolve_available_font_family,
};
pub use markdown_font_size::{
    MARKDOWN_FONT_SIZE_DEFAULT, MARKDOWN_FONT_SIZE_MAX, MARKDOWN_FONT_SIZE_MIN,
    normalize_markdown_font_size,
};
pub use normalization::{
    migrate_theme_mode_from_sync_system_theme, normalize_editor_engine,
    normalize_editor_line_height, normalize_external_editor, normalize_file_tree_indent_size,
    normalize_render_whitespace, normalize_settings, normalize_theme, normalize_theme_mode_value,
    normalize_workspace_keep_alive_minutes,
};
pub use raw_value::RawNumber;
pub use types::{
    CoreFeaturesState, EditorEngine, ExternalEditorMode, FileTreeDensity, RenderWhitespaceMode,
    Settings, SidebarPosition, TerminalCursorStyle, ThemeMode, normalize_file_tree_density,
};
pub use ui_font_size::{
    UI_FONT_SIZE_DEFAULT, UI_FONT_SIZE_MAX, UI_FONT_SIZE_MIN, format_ui_font_size,
    get_ui_font_scale, normalize_ui_font_size, shift_ui_font_size,
};
