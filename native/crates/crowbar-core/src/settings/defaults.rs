//! `defaultSettings` + `getDefaultSetting`/`getDefaultSettingsSnapshot`.
//!
//! Ported from `web/src/features/settings/config/default-settings.ts`.
//!
//! # Two TS exports with no direct Rust counterpart, and why
//!
//! * **`getDefaultSetting<K>(key: K): Settings[K]`** is a generic keyed
//!   accessor (`defaultSettings[key]`). Rust has no ergonomic equivalent of
//!   TS's `K extends keyof Settings` type-level dispatch without a
//!   hand-rolled enum-of-every-field-name that would just re-encode a plain
//!   field access with no behaviour of its own. [`default_settings`]`().foo`
//!   is the idiomatic Rust spelling of `getDefaultSetting('foo')` — direct
//!   field access on the struct this function already returns.
//! * **`getDefaultSettingsSnapshot(): Settings`** exists to defend against a
//!   JS-specific hazard this port does not have: `{...defaultSettings}` is a
//!   *shallow* copy, so two "snapshots" taken via a plain spread would share
//!   the same `coreFeatures` object and `hiddenFilePatterns`/
//!   `hiddenDirectoryPatterns` arrays by reference — mutating one snapshot's
//!   array would corrupt every other snapshot and `defaultSettings` itself.
//!   `getDefaultSettingsSnapshot` re-spreads those three fields specifically
//!   to force a deep-enough copy. In Rust, every [`Settings`] field is
//!   owned (`String`, `Vec<String>`, a plain `Copy` struct for
//!   `core_features`) — [`default_settings`] returning by value (or a
//!   `.clone()` of a stored one) is *already* fully independent; there is no
//!   shared-reference hazard for a second function to defend against. This
//!   is the same category of "the type system already rules out the bug"
//!   finding `crate::git::normalize_diff`'s module doc makes for a different
//!   field.
//!
//!   The function's *other* job — re-running `uiFontSize`/`markdownFontSize`
//!   through their own clamps, defensively, even though the literal defaults
//!   already satisfy them — ports as an invariant test instead
//!   ([`default_font_sizes_are_already_fixed_points_of_their_own_clamp`],
//!   below): proof that [`default_settings`]'s font-size fields need no
//!   correction, which is the fact the TS function's redundant call was
//!   actually establishing.

use super::types::{
    CoreFeaturesState, EditorEngine, ExternalEditorMode, FileTreeDensity, RenderWhitespaceMode,
    Settings, SidebarPosition, TerminalCursorStyle, ThemeMode,
};
use super::typography::{
    DEFAULT_CODE_FONT_SIZE, DEFAULT_MONO_FONT_FAMILY, DEFAULT_TERMINAL_FONT_FAMILY,
    DEFAULT_TERMINAL_FONT_SIZE, DEFAULT_UI_FONT_FAMILY,
};

/// `defaultSettings.theme`. Named separately (rather than only reachable via
/// `default_settings().theme`) because
/// [`super::normalization::normalize_theme`] also needs it as a fallback
/// value, matching `getDefaultSetting('theme')`'s own reuse in the TS
/// source's `normalizeTheme`.
pub const DEFAULT_THEME_ID: &str = "crowbar";

/// Mirrors `defaultSettings`.
#[must_use]
pub fn default_settings() -> Settings {
    Settings {
        // General
        auto_save: false,
        sidebar_position: SidebarPosition::Left,
        // Editor
        font_family: DEFAULT_MONO_FONT_FAMILY.to_string(),
        editor_engine: EditorEngine::Monaco,
        font_size: DEFAULT_CODE_FONT_SIZE,
        markdown_font_size: super::markdown_font_size::MARKDOWN_FONT_SIZE_DEFAULT,
        editor_line_height: 1.4,
        tab_size: 2.0,
        word_wrap: false,
        line_numbers: true,
        render_whitespace: RenderWhitespaceMode::None,
        render_indent_guides: true,
        semantic_highlighting: true,
        highlight_occurrences: false,
        show_minimap: false,
        // Terminal
        terminal_font_family: DEFAULT_TERMINAL_FONT_FAMILY.to_string(),
        terminal_font_size: DEFAULT_TERMINAL_FONT_SIZE,
        terminal_line_height: 1.0,
        terminal_letter_spacing: 0.0,
        terminal_scrollback: 10000.0,
        terminal_cursor_style: TerminalCursorStyle::Block,
        terminal_cursor_blink: true,
        terminal_cursor_width: 2.0,
        // UI
        ui_font_family: DEFAULT_UI_FONT_FAMILY.to_string(),
        ui_font_size: super::ui_font_size::UI_FONT_SIZE_DEFAULT,
        // Theme
        theme: DEFAULT_THEME_ID.to_string(),
        icon_theme: "material".to_string(),
        theme_mode: ThemeMode::System,
        sync_system_theme: false,
        auto_theme_light: "crowbar-light".to_string(),
        auto_theme_dark: "crowbar-dark".to_string(),
        window_transparency: true,
        // Layout
        sidebar_width: 220.0,
        // Language
        format_on_save: false,
        formatter: "prettier".to_string(),
        lint_on_save: false,
        auto_completion: true,
        parameter_hints: true,
        // External Editor
        external_editor: ExternalEditorMode::None,
        custom_editor_command: String::new(),
        // Features
        core_features: CoreFeaturesState { breadcrumbs: true },
        // Advanced
        show_fps_overlay: false,
        workspace_keep_alive_minutes: 10.0,
        // Other
        max_open_tabs: 100.0,
        // File tree
        file_tree_indent_size: 16.0,
        compact_folders_in_file_tree: false,
        file_tree_density: FileTreeDensity::Default,
        show_hidden_files_in_file_tree: true,
        show_gitignored_files_in_file_tree: true,
        hidden_file_patterns: Vec::new(),
        hidden_directory_patterns: Vec::new(),
        show_git_status_in_file_tree: true,
        compact_git_status_badges: false,
    }
}

#[cfg(test)]
mod tests {
    use super::default_settings;
    use crate::settings::markdown_font_size::normalize_markdown_font_size;
    use crate::settings::raw_value::RawNumber;
    use crate::settings::ui_font_size::normalize_ui_font_size;

    fn assert_close(got: f64, want: f64) {
        assert!((got - want).abs() < 1e-9, "got {got}, want {want}");
    }

    // --- new: not exercised by the TS suite as a standalone test (the TS
    //     source runs this defensively inside getDefaultSettingsSnapshot,
    //     with no dedicated assertion of its own — see this module's doc) ---

    #[test]
    fn default_font_sizes_are_already_fixed_points_of_their_own_clamp() {
        let defaults = default_settings();
        assert_close(
            normalize_ui_font_size(Some(RawNumber::Number(defaults.ui_font_size))),
            defaults.ui_font_size,
        );
        assert_close(
            normalize_markdown_font_size(Some(RawNumber::Number(defaults.markdown_font_size))),
            defaults.markdown_font_size,
        );
    }

    #[test]
    fn two_independently_constructed_defaults_do_not_share_the_collection_fields() {
        // The exact hazard getDefaultSettingsSnapshot's re-spread defended
        // against in JS: mutate one snapshot's Vec/struct field and prove
        // the other is untouched.
        let mut a = default_settings();
        let b = default_settings();
        a.hidden_file_patterns.push("*.log".to_string());
        a.core_features.breadcrumbs = false;
        assert!(b.hidden_file_patterns.is_empty());
        assert!(b.core_features.breadcrumbs);
    }
}
