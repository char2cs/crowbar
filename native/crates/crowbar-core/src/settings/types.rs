//! Settings domain types.
//!
//! Ported from `web/src/features/settings/types/settings.ts` (the `Settings`
//! interface, ~50 fields) and `types/feature.ts` (`CoreFeaturesState`) —
//! combined into one file, matching `crate::keymap::types`'s own precedent
//! of co-locating a small cross-referenced type with the struct that embeds
//! it rather than giving a 3-line file its own module.
//!
//! # Shape differences from the TS source, and why
//!
//! * **Every TS `number` field is `f64`.** JS has exactly one numeric type,
//!   so `tabSize`, `terminalScrollback`, `maxOpenTabs` etc. are `number` in
//!   TS regardless of being conceptually integer counts. Splitting these
//!   into `f64` vs `i64`/`u32` per field would invent a distinction the TS
//!   schema itself never draws — kept uniform instead.
//! * **Six closed-domain fields are Rust enums, not `String`:**
//!   [`SidebarPosition`], [`EditorEngine`], [`RenderWhitespaceMode`],
//!   [`TerminalCursorStyle`], [`ThemeMode`], [`ExternalEditorMode`],
//!   [`FileTreeDensity`]. TS types these as closed string unions too, but a
//!   TS union is a compile-time-only promise — nothing stops an `as`-cast
//!   persisted value from holding a string outside the union at runtime,
//!   which is `settings-normalization.ts`'s entire reason for existing (see
//!   `super::normalization`'s module doc for the full account of where that
//!   leaves this port's `normalize_settings`, and why several of its TS test
//!   cases relocate to standalone `parse_*`/`normalize_*` functions that
//!   accept a raw `&str` instead). This crate follows
//!   `crate::keymap::types`'s own precedent here (`CommandCategory`,
//!   `KeymapPresetId`) — same reasoning, same trade-off.
//! * **`theme`, `iconTheme`, `autoThemeLight`, `autoThemeDark` stay
//!   `String`.** Unlike the six enums above, TS types `Theme` as a bare
//!   `string` — there is no closed union to seal, because the valid set is
//!   a *runtime* registry (`theme-registry.ts`'s `ThemeRegistry`, built-ins
//!   plus session-uploaded custom themes), not a compile-time-fixed list.
//!   See [`super::normalization::normalize_theme`]'s doc for how this port
//!   handles that dynamic-registry dependency without pulling a mutable
//!   global into this crate (§4.3 rule 1 forbids `gpui`, but a
//!   `gpui`-free stateful singleton would still be a purity smell this
//!   crate's other modules avoid).

/// Mirrors `types/feature.ts`'s `CoreFeaturesState`.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct CoreFeaturesState {
    pub breadcrumbs: bool,
}

/// `Settings['sidebarPosition']` (`'left' | 'right'`). Not touched by
/// `settings-normalization.ts` — sealed here purely for the same
/// invalid-states-unrepresentable reasoning as the other five enums, not
/// because any ported function validates it.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum SidebarPosition {
    Left,
    Right,
}

/// `Settings['editorEngine']` (`'monaco' | 'nvim' | 'helix' | 'vim' |
/// 'custom'`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum EditorEngine {
    Monaco,
    Nvim,
    Helix,
    Vim,
    Custom,
}

/// `Settings['renderWhitespace']` (`RenderWhitespaceMode` in the TS source:
/// `'none' | 'boundary' | 'trailing' | 'all'`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum RenderWhitespaceMode {
    None,
    Boundary,
    Trailing,
    All,
}

/// `Settings['terminalCursorStyle']` (`'block' | 'underline' | 'bar'`). Not
/// touched by `settings-normalization.ts` — sealed for the same reason as
/// [`SidebarPosition`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum TerminalCursorStyle {
    Block,
    Underline,
    Bar,
}

/// `Settings['themeMode']` (`ThemeMode` in the TS source: `'light' | 'dark'
/// | 'system'`).
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ThemeMode {
    Light,
    Dark,
    System,
}

/// `Settings['externalEditor']` (`'none' | 'nvim' | 'helix' | 'vim' |
/// 'custom'`) — a distinct type from [`EditorEngine`] even though 4 of 5
/// variants share a spelling: TS declares this as its own inline union
/// (`'none' | 'nvim' | 'helix' | 'vim' | 'custom'`), not a reuse of
/// `EditorEngine` (which has `'monaco'` where this has `'none'`). See
/// [`super::normalization::normalize_external_editor`] for how the "external
/// editor overrides the editor engine" cross-field rule maps one onto the
/// other for exactly the 4 shared variants.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExternalEditorMode {
    None,
    Nvim,
    Helix,
    Vim,
    Custom,
}

/// `Settings['fileTreeDensity']` (`'compact' | 'default' | 'comfortable'`).
///
/// **Reconciled (P3.75).** This was, until the file-tree-model item, a
/// narrow, deliberately-flagged local duplicate of `web/src/features/
/// file-explorer/lib/file-tree-density.ts`'s own type — needed because
/// `settings-normalization.ts` imports `normalizeFileTreeDensity` directly
/// and calls it unconditionally in the boot-time `normalizeSettings` path
/// (§4's own Liveness section), so this crate needed *a* `FileTreeDensity`
/// type and normalizer to compile before file-tree model was itself ported.
/// That item has now landed: [`crate::file_tree::density::FileTreeDensity`]
/// is the one definition in the crate, and this is a re-export of it, not a
/// second declaration — see that module's doc for the full reconciliation
/// account, matching `crate::git`'s module doc precedent for the analogous
/// git-model/`crowbar-proto` reuse situation.
pub use crate::file_tree::density::FileTreeDensity;

/// Mirrors `types/settings.ts`'s `Settings` interface — the schema itself,
/// ~50 fields spanning general/editor/terminal/UI/theme/layout/language/
/// file-tree settings.
///
/// `clippy::struct_excessive_bools` (pedantic) flags this struct's ~20
/// independent `bool` fields and suggests a state machine or two-variant
/// enums. The TS source's own schema has exactly this many independent
/// booleans — `autoSave`, `wordWrap`, `lineNumbers`, `showMinimap`, and so
/// on are genuinely unrelated toggles, not encodings of a shared state
/// machine, and none of them are ever validated or clamped against each
/// other (`super::normalization` touches none of them). Inventing ~20
/// bespoke two-variant enums to satisfy the lint would add indirection this
/// schema's own shape does not call for, so it is allowed here rather than
/// worked around.
#[derive(Debug, Clone, PartialEq)]
#[allow(clippy::struct_excessive_bools)]
pub struct Settings {
    // General
    pub auto_save: bool,
    pub sidebar_position: SidebarPosition,
    // Editor
    pub font_family: String,
    pub editor_engine: EditorEngine,
    pub font_size: f64,
    /// Base type size for the rich markdown editor — see
    /// [`super::markdown_font_size`].
    pub markdown_font_size: f64,
    pub editor_line_height: f64,
    pub tab_size: f64,
    pub word_wrap: bool,
    pub line_numbers: bool,
    pub render_whitespace: RenderWhitespaceMode,
    pub render_indent_guides: bool,
    pub semantic_highlighting: bool,
    pub highlight_occurrences: bool,
    pub show_minimap: bool,
    // Terminal
    pub terminal_font_family: String,
    pub terminal_font_size: f64,
    pub terminal_line_height: f64,
    pub terminal_letter_spacing: f64,
    pub terminal_scrollback: f64,
    pub terminal_cursor_style: TerminalCursorStyle,
    pub terminal_cursor_blink: bool,
    pub terminal_cursor_width: f64,
    // UI
    pub ui_font_family: String,
    pub ui_font_size: f64,
    // Theme
    pub theme: String,
    pub icon_theme: String,
    pub theme_mode: ThemeMode,
    /// Deprecated — kept for migration only, matching the TS source's own
    /// comment on this field.
    pub sync_system_theme: bool,
    /// Deprecated — kept for migration only.
    pub auto_theme_light: String,
    /// Deprecated — kept for migration only.
    pub auto_theme_dark: String,
    pub window_transparency: bool,
    // Layout
    pub sidebar_width: f64,
    // Language
    pub format_on_save: bool,
    pub formatter: String,
    pub lint_on_save: bool,
    pub auto_completion: bool,
    pub parameter_hints: bool,
    // External Editor
    pub external_editor: ExternalEditorMode,
    pub custom_editor_command: String,
    // Features
    pub core_features: CoreFeaturesState,
    // Advanced
    pub show_fps_overlay: bool,
    /// How long (minutes) a workspace stays mounted in memory after you
    /// switch away, so switching back is instant. `0` destroys it on switch
    /// (the old behaviour). Capped at `crate::workspace::keep_alive::RETENTION_CAP`
    /// workspaces regardless of this value — a different cap from the one
    /// this field's own clamp range enforces; see
    /// `super::normalization::normalize_workspace_keep_alive_minutes`.
    pub workspace_keep_alive_minutes: f64,
    // Other
    pub max_open_tabs: f64,
    // File tree
    pub file_tree_indent_size: f64,
    pub compact_folders_in_file_tree: bool,
    pub file_tree_density: FileTreeDensity,
    pub show_hidden_files_in_file_tree: bool,
    pub show_gitignored_files_in_file_tree: bool,
    pub hidden_file_patterns: Vec<String>,
    pub hidden_directory_patterns: Vec<String>,
    pub show_git_status_in_file_tree: bool,
    pub compact_git_status_badges: bool,
}

/// Re-export of [`crate::file_tree::density::normalize_file_tree_density`]
/// — see [`FileTreeDensity`]'s doc. Tests for this function now live in
/// `crate::file_tree::density`, the module that owns the definition; this
/// file no longer carries its own copy of them.
pub use crate::file_tree::density::normalize_file_tree_density;
