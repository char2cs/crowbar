//! Validation, clamping and migration across ~15 fields — the substance of
//! the settings-schema area.
//!
//! Ported from `web/src/features/settings/lib/settings-normalization.ts`.
//! This is the file `main.tsx` → `settings-bootstrap.ts` calls unconditionally
//! at boot (`native/mapping/tier-a-denominator.md` §4's Liveness section), on
//! a `Settings` value that was just cast, unsafely, out of whatever JSON
//! `settings-persistence.ts`'s `localStorage` read produced
//! (`value as Settings[typeof key]`, per that file's own source — not
//! ported, D6). So the TS `normalizeSettings`/`normalizeSettingValue`
//! genuinely run on data the compiler *believes* is a well-formed `Settings`
//! but that, at runtime, might not be: a missing field from before it
//! existed, a since-removed enum value, a legacy default that needs
//! migrating forward.
//!
//! # Why this file's functions don't all take `&Settings`
//!
//! [`super::types::Settings`] seals six fields as Rust enums
//! ([`super::types::Settings`]'s own module doc explains why) — which means,
//! by construction, a `Settings` value can never actually hold the malformed
//! shapes several of the ported TS test cases construct on purpose (e.g.
//! `editorEngine: 'emacs' as never`, `themeMode: undefined as unknown as
//! ThemeMode`). This is the *exact* situation `crate::git::normalize_diff`'s
//! module doc already worked through for a different field: **"the field
//! can't lie about its own shape … there is no runtime state a normalize
//! function could observe and repair that the type doesn't already rule
//! out."** Two of the ported TS behaviours are genuinely unreachable once a
//! `Settings` is already constructed, for exactly that reason — see
//! [`normalize_settings`]'s own doc for which two, and why their ported test
//! cases relocate to the standalone `parse_*`/`normalize_*` functions in this
//! file instead of `normalize_settings` itself. Every other clamp/migration
//! *is* still meaningful on an already-typed `Settings` (an `f64` field can
//! still hold `NaN`/`Infinity`/an out-of-range value — nothing about `f64`
//! rules that out — and `theme: String` has no seal at all, by design; see
//! `types`'s module doc) and is ported as a method on the whole struct,
//! matching the TS shape exactly.
//!
//! # Four asymmetries in the original source, found while enumerating
//! # every branch (per this item's own verification requirement) and
//! # preserved rather than "fixed"
//!
//! 1. **`normalizeFileTreeIndentSize`'s non-finite fallback is `20`, not
//!    the schema default of `16`.** `defaultSettings.fileTreeIndentSize`
//!    is `16`; the clamp function's own `!Number.isFinite(value)` branch
//!    returns a literal `20` instead. These are two independently-declared
//!    numbers in the TS source with no shared constant between them — most
//!    likely a bug (a NaN-shaped persisted value migrates to a value the
//!    user never chose and the schema doesn't call "default"), but it is
//!    the *actual, live* behaviour, so [`normalize_file_tree_indent_size`]
//!    reproduces `20` exactly, with a test proving it (`fallback_is_20…`,
//!    below) — not `16`.
//! 2. **`normalizeEditorEngine`'s `customEditorCommand` parameter is
//!    unused.** The TS signature is `normalizeEditorEngine(value: unknown,
//!    _customEditorCommand: string | undefined)` — underscore-prefixed,
//!    dead. The real "a `custom` engine with no command string falls back
//!    to `monaco`" rule lives entirely in `normalizeSettings`'s own body,
//!    applied *after* this function returns, not inside it. So
//!    [`normalize_editor_engine`] here takes no `custom_editor_command`
//!    parameter at all; that cross-field rule is [`normalize_settings`]'s
//!    job, matching where the TS source actually puts it.
//! 3. **`externalEditor` has no branch in `normalizeSettingValue` at all.**
//!    Every other enum-ish field (`renderWhitespace`, `editorEngine`,
//!    `fileTreeDensity`, `theme`, `themeMode`) has a `key === '…'` branch in
//!    `normalizeSettingValue`; `externalEditor` does not, so a direct
//!    per-key update of that setting is never validated — only a full
//!    `normalizeSettings` pass (e.g. at next boot) would catch a bad value.
//!    Not closed here either: inventing a per-key entry point this port
//!    doesn't otherwise have, to validate a field the TS source itself never
//!    validates that way, would be adding behaviour instead of porting it.
//! 4. **`themeMode` has two unrelated normalizers, not one.**
//!    `normalizeSettings`'s inline block only checks *falsiness*
//!    (`!normalizedSettings.themeMode`) and, when falsy, derives from
//!    `syncSystemTheme` — it does **not** validate an already-truthy value,
//!    so `normalizeSettings({..., themeMode: 'invalid'})` leaves `'invalid'`
//!    untouched in the TS source. `normalizeSettingValue('themeMode', …)`,
//!    by contrast, validates membership (`['light','dark','system'].includes(...)`)
//!    and falls back to `'system'` — but never consults `syncSystemTheme`.
//!    These port as two distinctly-named functions,
//!    [`migrate_theme_mode_from_sync_system_theme`] and
//!    [`normalize_theme_mode_value`], rather than one merged function that
//!    would silently change which cases each behaviour actually covers.

use super::defaults::DEFAULT_THEME_ID;
use super::font_family::normalize_configured_font_family;
use super::markdown_font_size::normalize_markdown_font_size;
use super::raw_value::RawNumber;
use super::types::{EditorEngine, ExternalEditorMode, RenderWhitespaceMode, Settings, ThemeMode};
use super::ui_font_size::normalize_ui_font_size;

const LEGACY_TERMINAL_LINE_HEIGHT_DEFAULT: f64 = 1.2;
const TERMINAL_LINE_HEIGHT_DEFAULT: f64 = 1.0;
const EDITOR_LINE_HEIGHT_MIN: f64 = 1.0;
const EDITOR_LINE_HEIGHT_MAX: f64 = 2.0;
/// Literal, not derived from a shared constant — matches the TS source's own
/// `!Number.isFinite(value) -> 1.4` branch, which is independently declared
/// from `defaultSettings.editorLineHeight` (also `1.4`, but a coincidence of
/// value, not a reference — unlike `fileTreeIndentSize`'s equivalent branch,
/// see this module's doc, item 1, where the two numbers actually diverge).
const EDITOR_LINE_HEIGHT_NON_FINITE_FALLBACK: f64 = 1.4;
const FILE_TREE_INDENT_SIZE_MIN: f64 = 8.0;
const FILE_TREE_INDENT_SIZE_MAX: f64 = 32.0;
/// See this module's doc, item 1: deliberately **not** `16`
/// (`defaultSettings.fileTreeIndentSize`).
const FILE_TREE_INDENT_SIZE_NON_FINITE_FALLBACK: f64 = 20.0;
const WORKSPACE_KEEP_ALIVE_MIN: f64 = 0.0;
const WORKSPACE_KEEP_ALIVE_MAX: f64 = 120.0;
const WORKSPACE_KEEP_ALIVE_DEFAULT: f64 = 10.0;

/// `value === LEGACY_TERMINAL_LINE_HEIGHT_DEFAULT` in the TS source — exact
/// equality on a literal, spelled with an epsilon so `clippy::float_cmp`
/// (pedantic, denied workspace-wide) can see it is deliberate rather than an
/// accidental `==` on a float, matching `crowbar-driver`'s `assert_px`
/// precedent for the same situation.
fn is_legacy_terminal_line_height(value: f64) -> bool {
    (value - LEGACY_TERMINAL_LINE_HEIGHT_DEFAULT).abs() < 1e-9
}

/// Mirrors `normalizeEditorLineHeight`. Non-finite (`NaN`/`±Infinity`) falls
/// back to [`EDITOR_LINE_HEIGHT_NON_FINITE_FALLBACK`]; otherwise the value is
/// snapped to one decimal place and clamped to `[1.0, 2.0]`.
#[must_use]
pub fn normalize_editor_line_height(value: f64) -> f64 {
    if !value.is_finite() {
        return EDITOR_LINE_HEIGHT_NON_FINITE_FALLBACK;
    }

    let snapped = (value * 10.0).round() / 10.0;
    snapped.clamp(EDITOR_LINE_HEIGHT_MIN, EDITOR_LINE_HEIGHT_MAX)
}

/// Mirrors `normalizeFileTreeIndentSize`. Non-finite falls back to
/// [`FILE_TREE_INDENT_SIZE_NON_FINITE_FALLBACK`] — **not** the schema
/// default; see this module's doc, item 1. Otherwise rounded to a whole
/// number and clamped to `[8, 32]`.
#[must_use]
pub fn normalize_file_tree_indent_size(value: f64) -> f64 {
    if !value.is_finite() {
        return FILE_TREE_INDENT_SIZE_NON_FINITE_FALLBACK;
    }

    let snapped = value.round();
    snapped.clamp(FILE_TREE_INDENT_SIZE_MIN, FILE_TREE_INDENT_SIZE_MAX)
}

/// Mirrors `normalizeWorkspaceKeepAliveMinutes(value: unknown)`. Unlike
/// [`super::ui_font_size::normalize_ui_font_size`]/
/// [`super::markdown_font_size::normalize_markdown_font_size`], this does
/// **not** parse a numeric string — the TS guard is `typeof value !==
/// 'number'`, a stricter check than those two functions' own `typeof value
/// === 'string' ? Number(value) : …`. A `RawNumber::Text` therefore falls
/// back to the default here, where it would parse for the font-size
/// functions.
#[must_use]
pub fn normalize_workspace_keep_alive_minutes(value: Option<RawNumber<'_>>) -> f64 {
    let Some(RawNumber::Number(n)) = value else {
        return WORKSPACE_KEEP_ALIVE_DEFAULT;
    };
    if !n.is_finite() {
        return WORKSPACE_KEEP_ALIVE_DEFAULT;
    }

    let snapped = n.round();
    snapped.clamp(WORKSPACE_KEEP_ALIVE_MIN, WORKSPACE_KEEP_ALIVE_MAX)
}

/// Mirrors `isRenderWhitespaceMode` + `normalizeRenderWhitespace` combined
/// (TS keeps these as two functions; ported as one, since — unlike
/// `renderWhitespace`'s TS split — nothing else in this crate needs the bare
/// membership check standalone). `None` (missing/non-string) or any string
/// outside the four known modes falls back to `RenderWhitespaceMode::None`
/// (`'none'`).
#[must_use]
pub fn normalize_render_whitespace(value: Option<&str>) -> RenderWhitespaceMode {
    match value {
        Some("boundary") => RenderWhitespaceMode::Boundary,
        Some("trailing") => RenderWhitespaceMode::Trailing,
        Some("all") => RenderWhitespaceMode::All,
        // `Some("none")` and every other input (missing, or a string
        // outside the four known modes) share the same fallback — clippy
        // rightly flags a redundant explicit `Some("none")` arm identical
        // to the wildcard.
        _ => RenderWhitespaceMode::None,
    }
}

/// Mirrors `normalizeEditorEngine`, minus its unused `customEditorCommand`
/// parameter — see this module's doc, item 2. Falls back to
/// `EditorEngine::Monaco` for anything outside the five known engines.
#[must_use]
pub fn normalize_editor_engine(value: Option<&str>) -> EditorEngine {
    match value {
        Some("nvim") => EditorEngine::Nvim,
        Some("helix") => EditorEngine::Helix,
        Some("vim") => EditorEngine::Vim,
        Some("custom") => EditorEngine::Custom,
        // `Some("monaco")` and everything else (missing, or a value the
        // schema no longer has) share the fallback — see the redundant-arm
        // note on `normalize_render_whitespace` above.
        _ => EditorEngine::Monaco,
    }
}

/// Mirrors `normalizeExternalEditor`. Falls back to `ExternalEditorMode::None`
/// for anything outside the five known modes, *and* for a `'custom'` value
/// whose paired `custom_editor_command` is empty/whitespace-only.
#[must_use]
pub fn normalize_external_editor(
    value: Option<&str>,
    custom_editor_command: &str,
) -> ExternalEditorMode {
    let parsed = match value {
        Some("none") => ExternalEditorMode::None,
        Some("nvim") => ExternalEditorMode::Nvim,
        Some("helix") => ExternalEditorMode::Helix,
        Some("vim") => ExternalEditorMode::Vim,
        Some("custom") => ExternalEditorMode::Custom,
        _ => return ExternalEditorMode::None,
    };

    if matches!(parsed, ExternalEditorMode::Custom) && custom_editor_command.trim().is_empty() {
        return ExternalEditorMode::None;
    }

    parsed
}

/// `externalEditor` -> `editorEngine`, for the 4 variants the two enums
/// share (see [`super::types::ExternalEditorMode`]'s doc). `None` means "no
/// override" (mirrors `externalEditor !== 'none'` gating the assignment in
/// the TS source), so the return type is `Option`, not `EditorEngine`.
#[must_use]
fn external_editor_as_engine_override(mode: ExternalEditorMode) -> Option<EditorEngine> {
    match mode {
        ExternalEditorMode::None => None,
        ExternalEditorMode::Nvim => Some(EditorEngine::Nvim),
        ExternalEditorMode::Helix => Some(EditorEngine::Helix),
        ExternalEditorMode::Vim => Some(EditorEngine::Vim),
        ExternalEditorMode::Custom => Some(EditorEngine::Custom),
    }
}

/// Mirrors `normalizeSettingValue`'s `themeMode` branch: validates
/// membership against the three known modes, falling back to
/// `ThemeMode::System`. Does **not** consult `syncSystemTheme` — see this
/// module's doc, item 4, and contrast with
/// [`migrate_theme_mode_from_sync_system_theme`].
#[must_use]
pub fn normalize_theme_mode_value(value: Option<&str>) -> ThemeMode {
    match value {
        Some("light") => ThemeMode::Light,
        Some("dark") => ThemeMode::Dark,
        // `Some("system")` and everything else (missing, or an invalid
        // mode) share the fallback — same redundant-arm situation as
        // `normalize_render_whitespace` above.
        _ => ThemeMode::System,
    }
}

/// Mirrors `normalizeSettings`'s inline `themeMode` block: `if
/// (!normalizedSettings.themeMode) { themeMode = syncSystemTheme ? 'system'
/// : 'light' }`. Checks **only** falsiness (`value` is `None` or `Some("")`)
/// — an already-present-but-invalid string is passed through unvalidated in
/// the TS source, which is why this returns the TS *string*, not a
/// [`super::types::ThemeMode`]: a caller with an already-sealed `ThemeMode`
/// has nothing for this function to observe (see this module's top-level
/// doc on why [`normalize_settings`] does not call this). Provided for the
/// boundary — whatever future deserialization step constructs a `Settings`
/// from raw, untrusted JSON is where this migration is actually reachable,
/// matching `crate::keymap::presets::parse_keymap_preset_id`'s precedent for
/// "the one call site that still needs to validate an untrusted string."
#[must_use]
pub fn migrate_theme_mode_from_sync_system_theme(
    value: Option<&str>,
    sync_system_theme: bool,
) -> ThemeMode {
    let is_falsy = value.is_none_or(str::is_empty);
    if !is_falsy {
        // Passed through unvalidated in the TS source — but this function's
        // return type is already the sealed enum, so "pass through
        // unvalidated" has no faithful representation for an invalid
        // string here. Every caller of this function in this port only
        // ever calls it with `None`/`Some("")` (see the tests below); a
        // future caller passing an already-valid string should prefer
        // `normalize_theme_mode_value` or construct the enum directly.
        return normalize_theme_mode_value(value);
    }

    if sync_system_theme {
        ThemeMode::System
    } else {
        ThemeMode::Light
    }
}

/// Mirrors `normalizeTheme`. `Theme` is a bare `string` in the TS source —
/// see `types`'s module doc for why this crate does not seal it — so the
/// "known ids" set is a caller-supplied parameter rather than a hardcoded
/// list or a reach into a mutable global registry
/// (`theme-registry.ts`'s `ThemeRegistry`, which also is not portable here:
/// it imports `appearance-bootstrap.ts`, explicitly out of scope, D6/
/// webview-only). `resolve_available_font_family` (`font_family.rs`)
/// already establishes this "caller supplies the known set" shape in this
/// same area, so this is not a new pattern for this port.
///
/// Falls back to `default_theme` for a `None` value or one absent from
/// `known_theme_ids`.
#[must_use]
pub fn normalize_theme(
    value: Option<&str>,
    known_theme_ids: &[&str],
    default_theme: &str,
) -> String {
    if let Some(v) = value
        && known_theme_ids.contains(&v)
    {
        return v.to_string();
    }

    default_theme.to_string()
}

/// Mirrors `normalizeSettings(settings: Settings): Settings`. See this
/// module's top-level doc for which fields this cannot meaningfully touch
/// once `settings` is already a valid, sealed `Settings` (renderWhitespace,
/// fileTreeDensity, and — see the themeMode paragraph below — themeMode),
/// and why: the TS behaviour those fields' branches exist to catch requires
/// an already-invalid value to observe, which this port's `Settings` cannot
/// hold by construction. Every other field's clamp/migration/cross-field
/// rule is applied exactly as the TS source does.
///
/// `known_theme_ids` is threaded through to [`normalize_theme`] — see its
/// doc for why this is a parameter rather than a global lookup.
///
/// # `themeMode`, specifically
///
/// The TS source's inline "derive from `syncSystemTheme` when falsy" block
/// has no reachable input here: `settings.theme_mode` is already a valid
/// `ThemeMode`, which can never be "falsy" the way `undefined`/`null`/`""`
/// are in TS. This is the exact shape `crate::git::normalize_diff`'s module
/// doc already documents for a different field: the migration's precondition
/// doesn't survive being typed. `theme_mode` is therefore passed through
/// unchanged; [`migrate_theme_mode_from_sync_system_theme`] carries the same
/// tested behaviour at the boundary where it's still reachable (see this
/// module's top-level doc, item 4).
#[must_use]
pub fn normalize_settings(settings: &Settings, known_theme_ids: &[&str]) -> Settings {
    let mut out = settings.clone();

    out.font_family = normalize_configured_font_family(
        &settings.font_family,
        super::typography::DEFAULT_MONO_FONT_FAMILY,
    );
    out.terminal_font_family = normalize_configured_font_family(
        &settings.terminal_font_family,
        super::typography::DEFAULT_MONO_FONT_FAMILY,
    );
    out.ui_font_family = normalize_configured_font_family(
        &settings.ui_font_family,
        super::typography::DEFAULT_UI_FONT_FAMILY,
    );

    out.ui_font_size = normalize_ui_font_size(Some(RawNumber::Number(settings.ui_font_size)));
    out.markdown_font_size =
        normalize_markdown_font_size(Some(RawNumber::Number(settings.markdown_font_size)));

    out.terminal_line_height = if is_legacy_terminal_line_height(settings.terminal_line_height) {
        TERMINAL_LINE_HEIGHT_DEFAULT
    } else {
        settings.terminal_line_height
    };
    out.editor_line_height = normalize_editor_line_height(settings.editor_line_height);

    // renderWhitespace / fileTreeDensity: sealed by construction, no-op —
    // see this function's doc.

    // Cross-field editor engine precedence, in the TS source's own order:
    // the custom-with-empty-command fallback first, then the
    // externalEditor override (which can override the fallback's own
    // result too, matching the TS source's sequential mutation).
    let mut editor_engine = settings.editor_engine;
    if matches!(editor_engine, EditorEngine::Custom)
        && settings.custom_editor_command.trim().is_empty()
    {
        editor_engine = EditorEngine::Monaco;
    }
    if let Some(overridden) = external_editor_as_engine_override(settings.external_editor) {
        editor_engine = overridden;
    }
    out.editor_engine = editor_engine;

    out.file_tree_indent_size = normalize_file_tree_indent_size(settings.file_tree_indent_size);
    out.workspace_keep_alive_minutes = normalize_workspace_keep_alive_minutes(Some(
        RawNumber::Number(settings.workspace_keep_alive_minutes),
    ));

    out.theme = normalize_theme(Some(&settings.theme), known_theme_ids, DEFAULT_THEME_ID);

    // themeMode: sealed by construction, no-op — see this function's doc.

    out
}

#[cfg(test)]
mod tests {
    use super::{
        ExternalEditorMode, RenderWhitespaceMode, ThemeMode,
        migrate_theme_mode_from_sync_system_theme, normalize_editor_engine,
        normalize_editor_line_height, normalize_external_editor, normalize_file_tree_indent_size,
        normalize_render_whitespace, normalize_settings, normalize_theme,
        normalize_theme_mode_value, normalize_workspace_keep_alive_minutes,
    };
    use crate::settings::defaults::default_settings;
    use crate::settings::raw_value::RawNumber;
    use crate::settings::types::EditorEngine;

    fn assert_close(got: f64, want: f64) {
        assert!((got - want).abs() < 1e-9, "got {got}, want {want}");
    }

    const KNOWN_THEMES: &[&str] = &["crowbar", "zen"];

    // === font settings, ported from settings-normalization.test.ts ===

    #[test]
    fn preserves_configured_font_settings_that_may_exist_on_the_system() {
        let mut settings = default_settings();
        settings.font_family = "\"Geist Mono\"".to_string();
        settings.terminal_font_family = "Geist Mono, monospace".to_string();
        settings.ui_font_family = "Geist".to_string();

        let normalized = normalize_settings(&settings, KNOWN_THEMES);
        assert_eq!(normalized.font_family, "\"Geist Mono\"");
        assert_eq!(normalized.terminal_font_family, "Geist Mono, monospace");
        assert_eq!(normalized.ui_font_family, "Geist");
    }

    // === workspaceKeepAliveMinutes: clamp boundaries + missing/invalid-type fallback ===

    #[test]
    fn workspace_keep_alive_minutes_round_trips_valid_values() {
        let mut settings = default_settings();
        settings.workspace_keep_alive_minutes = 0.0;
        assert_close(
            normalize_settings(&settings, KNOWN_THEMES).workspace_keep_alive_minutes,
            0.0,
        );

        settings.workspace_keep_alive_minutes = 45.0;
        assert_close(
            normalize_settings(&settings, KNOWN_THEMES).workspace_keep_alive_minutes,
            45.0,
        );
    }

    #[test]
    fn workspace_keep_alive_minutes_clamps_below_at_and_above_the_range() {
        // below
        assert_close(
            normalize_workspace_keep_alive_minutes(Some(RawNumber::Number(-5.0))),
            0.0,
        );
        // at the boundary — must survive unclamped
        assert_close(
            normalize_workspace_keep_alive_minutes(Some(RawNumber::Number(0.0))),
            0.0,
        );
        assert_close(
            normalize_workspace_keep_alive_minutes(Some(RawNumber::Number(120.0))),
            120.0,
        );
        // above
        assert_close(
            normalize_workspace_keep_alive_minutes(Some(RawNumber::Number(9999.0))),
            120.0,
        );
    }

    #[test]
    fn workspace_keep_alive_minutes_rounds_a_fractional_value() {
        assert_close(
            normalize_workspace_keep_alive_minutes(Some(RawNumber::Number(10.6))),
            11.0,
        );
    }

    #[test]
    fn workspace_keep_alive_minutes_falls_back_to_default_for_nan_missing_or_a_string() {
        let mut settings = default_settings();
        settings.workspace_keep_alive_minutes = f64::NAN;
        assert_close(
            normalize_settings(&settings, KNOWN_THEMES).workspace_keep_alive_minutes,
            10.0,
        );

        assert_close(normalize_workspace_keep_alive_minutes(None), 10.0);
        // Unlike ui/markdown font size, a numeric STRING is rejected too —
        // this is the behaviour item 3 (this module's own asymmetry list,
        // no — see the function doc) singles out: `typeof value !==
        // 'number'` rejects strings outright.
        assert_close(
            normalize_workspace_keep_alive_minutes(Some(RawNumber::Text("45"))),
            10.0,
        );
    }

    // === terminalLineHeight: exact-value migration ===

    #[test]
    fn migrates_the_old_terminal_line_height_default_to_preserve_tui_block_graphics() {
        let mut settings = default_settings();
        settings.terminal_line_height = 1.2;
        assert_close(
            normalize_settings(&settings, KNOWN_THEMES).terminal_line_height,
            1.0,
        );
    }

    #[test]
    fn a_terminal_line_height_that_merely_resembles_the_legacy_value_is_not_migrated() {
        // Exact-value migration, not a range: 1.2000001 must survive
        // untouched, proving this isn't secretly a clamp/round.
        let mut settings = default_settings();
        settings.terminal_line_height = 1.2000_001;
        assert_close(
            normalize_settings(&settings, KNOWN_THEMES).terminal_line_height,
            1.2000_001,
        );
    }

    // === editorLineHeight: clamp boundaries ===

    #[test]
    fn editor_line_height_clamps_below_at_and_above_the_supported_range() {
        assert_close(normalize_editor_line_height(0.6), 1.0);
        assert_close(normalize_editor_line_height(1.0), 1.0);
        assert_close(normalize_editor_line_height(2.0), 2.0);
        assert_close(normalize_editor_line_height(2.6), 2.0);
        assert_close(normalize_editor_line_height(1.34), 1.3);
    }

    #[test]
    fn editor_line_height_falls_back_for_non_finite_input() {
        assert_close(normalize_editor_line_height(f64::NAN), 1.4);
        assert_close(normalize_editor_line_height(f64::INFINITY), 1.4);
    }

    // === fileTreeIndentSize: clamp boundaries + the 20-vs-16 fallback finding ===

    #[test]
    fn file_tree_indent_size_clamps_below_at_and_above_the_supported_range() {
        assert_close(normalize_file_tree_indent_size(2.0), 8.0);
        assert_close(normalize_file_tree_indent_size(8.0), 8.0);
        assert_close(normalize_file_tree_indent_size(32.0), 32.0);
        assert_close(normalize_file_tree_indent_size(40.0), 32.0);
        assert_close(normalize_file_tree_indent_size(13.6), 14.0);
    }

    #[test]
    fn file_tree_indent_size_non_finite_fallback_is_20_not_the_schema_default_of_16() {
        assert_close(normalize_file_tree_indent_size(f64::NAN), 20.0);
        // The schema default is 16, not 20 — proving the fallback above is
        // genuinely a different number, not a restatement of the default.
        assert_close(default_settings().file_tree_indent_size, 16.0);
    }

    // === editorEngine: valid-value fallback + the "schema no longer has it" case ===

    #[test]
    fn disables_blank_custom_editor_engine_settings() {
        let mut settings = default_settings();
        settings.editor_engine = EditorEngine::Custom;
        settings.custom_editor_command = String::new();
        assert_eq!(
            normalize_settings(&settings, KNOWN_THEMES).editor_engine,
            EditorEngine::Monaco
        );
    }

    #[test]
    fn a_custom_engine_with_a_real_command_is_kept() {
        let mut settings = default_settings();
        settings.editor_engine = EditorEngine::Custom;
        settings.custom_editor_command = "my-editor --wait".to_string();
        assert_eq!(
            normalize_settings(&settings, KNOWN_THEMES).editor_engine,
            EditorEngine::Custom
        );
    }

    #[test]
    fn normalize_editor_engine_falls_back_to_monaco_for_a_value_the_schema_no_longer_has() {
        // The TS test constructs `editorEngine: 'emacs' as never` — a value
        // outside the schema entirely — directly on a `Settings` object and
        // runs it through `normalizeSettings`. A sealed `EditorEngine` makes
        // that exact input unconstructible in Rust (see this module's top
        // doc), so the equivalent case moves to the boundary function that
        // actually receives an untrusted string.
        assert_eq!(normalize_editor_engine(Some("emacs")), EditorEngine::Monaco);
        assert_eq!(normalize_editor_engine(None), EditorEngine::Monaco);
    }

    #[test]
    fn migrates_legacy_external_editor_settings_into_editor_engine() {
        let mut settings = default_settings();
        settings.editor_engine = EditorEngine::Monaco;
        settings.external_editor = ExternalEditorMode::Helix;
        assert_eq!(
            normalize_settings(&settings, KNOWN_THEMES).editor_engine,
            EditorEngine::Helix
        );
    }

    #[test]
    fn every_external_editor_mode_overrides_editor_engine_to_its_matching_variant() {
        // The Helix case above exercises one of the four shared variants;
        // this covers the other three so `external_editor_as_engine_override`'s
        // full match is proven, not just its first reachable arm.
        for (external, expected) in [
            (ExternalEditorMode::Nvim, EditorEngine::Nvim),
            (ExternalEditorMode::Vim, EditorEngine::Vim),
            (ExternalEditorMode::Custom, EditorEngine::Custom),
        ] {
            let mut settings = default_settings();
            settings.editor_engine = EditorEngine::Monaco;
            settings.external_editor = external;
            // Custom needs a non-empty command or the custom-with-no-command
            // fallback would immediately overwrite the very override this
            // test is proving.
            settings.custom_editor_command = "code --wait".to_string();
            assert_eq!(
                normalize_settings(&settings, KNOWN_THEMES).editor_engine,
                expected
            );
        }
    }

    #[test]
    fn normalize_external_editor_falls_back_for_an_unknown_mode_and_for_custom_with_no_command() {
        assert_eq!(
            normalize_external_editor(Some("emacs"), ""),
            ExternalEditorMode::None
        );
        assert_eq!(
            normalize_external_editor(Some("custom"), "   "),
            ExternalEditorMode::None
        );
        assert_eq!(
            normalize_external_editor(Some("custom"), "code --wait"),
            ExternalEditorMode::Custom
        );
    }

    // === renderWhitespace: boundary function ===

    #[test]
    fn normalize_render_whitespace_accepts_all_four_known_modes_and_rejects_the_rest() {
        assert_eq!(
            normalize_render_whitespace(Some("boundary")),
            RenderWhitespaceMode::Boundary
        );
        assert_eq!(
            normalize_render_whitespace(Some("trailing")),
            RenderWhitespaceMode::Trailing
        );
        assert_eq!(
            normalize_render_whitespace(Some("all")),
            RenderWhitespaceMode::All
        );
        assert_eq!(
            normalize_render_whitespace(Some("none")),
            RenderWhitespaceMode::None
        );
        assert_eq!(
            normalize_render_whitespace(Some("garbage")),
            RenderWhitespaceMode::None
        );
        assert_eq!(
            normalize_render_whitespace(None),
            RenderWhitespaceMode::None
        );
    }

    // === themeMode: the two-function asymmetry (item 4) ===

    #[test]
    fn sets_theme_mode_to_system_when_sync_system_theme_was_true_and_the_value_is_missing() {
        assert_eq!(
            migrate_theme_mode_from_sync_system_theme(None, true),
            ThemeMode::System
        );
    }

    #[test]
    fn sets_theme_mode_to_light_when_sync_system_theme_was_false_and_the_value_is_missing() {
        assert_eq!(
            migrate_theme_mode_from_sync_system_theme(None, false),
            ThemeMode::Light
        );
    }

    #[test]
    fn an_empty_string_theme_mode_is_also_treated_as_missing() {
        assert_eq!(
            migrate_theme_mode_from_sync_system_theme(Some(""), true),
            ThemeMode::System
        );
    }

    #[test]
    fn a_present_theme_mode_is_not_migrated_regardless_of_sync_system_theme() {
        // The falsiness-only check's other branch: once `value` is
        // non-empty, `syncSystemTheme` plays no role at all — flipping it
        // here must not change the result, proving the migration truly
        // never fires for a truthy value.
        assert_eq!(
            migrate_theme_mode_from_sync_system_theme(Some("dark"), true),
            ThemeMode::Dark
        );
        assert_eq!(
            migrate_theme_mode_from_sync_system_theme(Some("dark"), false),
            ThemeMode::Dark
        );
    }

    #[test]
    fn preserves_existing_theme_mode_through_a_full_normalize_settings_pass() {
        let mut settings = default_settings();
        settings.theme_mode = ThemeMode::Dark;
        assert_eq!(
            normalize_settings(&settings, KNOWN_THEMES).theme_mode,
            ThemeMode::Dark
        );
    }

    #[test]
    fn normalize_theme_mode_value_rejects_an_invalid_mode_and_falls_back_to_system() {
        // This is `normalizeSettingValue('themeMode', 'invalid')`'s ported
        // behaviour — membership validation, no syncSystemTheme involved —
        // distinct from `migrate_theme_mode_from_sync_system_theme` above.
        assert_eq!(
            normalize_theme_mode_value(Some("invalid")),
            ThemeMode::System
        );
        assert_eq!(normalize_theme_mode_value(Some("dark")), ThemeMode::Dark);
    }

    // === theme: registry-coercion migration, ported from settings-normalization-theme.test.ts ===

    #[test]
    fn coerces_a_theme_id_the_registry_does_not_know_to_the_default_theme() {
        let mut settings = default_settings();
        settings.theme = "terra".to_string();
        assert_eq!(normalize_settings(&settings, KNOWN_THEMES).theme, "crowbar");
    }

    #[test]
    fn keeps_a_registered_theme_id_untouched() {
        let mut settings = default_settings();
        settings.theme = "zen".to_string();
        assert_eq!(normalize_settings(&settings, KNOWN_THEMES).theme, "zen");

        settings.theme = "crowbar".to_string();
        assert_eq!(normalize_settings(&settings, KNOWN_THEMES).theme, "crowbar");
    }

    #[test]
    fn coerces_a_missing_persisted_theme() {
        // The TS test casts `theme: undefined as unknown as string` — a
        // `Settings.theme: String` field can't hold "missing" the way a
        // `String` can't be `undefined`, so this exercises the same
        // boundary at the function that actually accepts `Option`.
        assert_eq!(normalize_theme(None, KNOWN_THEMES, "crowbar"), "crowbar");
    }

    #[test]
    fn coerces_an_unknown_theme_on_write_too() {
        assert_eq!(
            normalize_theme(Some("terra"), KNOWN_THEMES, "crowbar"),
            "crowbar"
        );
        assert_eq!(normalize_theme(Some("zen"), KNOWN_THEMES, "crowbar"), "zen");
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn empty_known_theme_ids_always_falls_back() {
        assert_eq!(normalize_theme(Some("crowbar"), &[], "crowbar"), "crowbar");
        assert_eq!(normalize_theme(Some("zen"), &[], "crowbar"), "crowbar");
    }

    #[test]
    fn font_family_settings_still_fall_back_when_blank_through_a_full_normalize_settings_pass() {
        let mut settings = default_settings();
        settings.font_family = "   ".to_string();
        assert_eq!(
            normalize_settings(&settings, KNOWN_THEMES).font_family,
            super::super::typography::DEFAULT_MONO_FONT_FAMILY
        );
    }

    // --- mutation testing note (per this port's verification requirement;
    //     see native/mapping/core-settings.md §6 for the full account) ---
    //
    // Five mutations were made against this file, one at a time, each
    // watched to fail for real, then reverted (`git status` clean
    // afterward). Real output, pasted verbatim:
    //
    // 1. Clamp boundary: `FILE_TREE_INDENT_SIZE_MAX` 32.0 -> 40.0.
    //
    //   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:417:9:
    //   got 40, want 32
    //   test result: FAILED. 1 passed; 1 failed; 0 ignored; 0 measured; 226 filtered out
    //
    //   (`file_tree_indent_size_clamps_below_at_and_above_the_supported_range`)
    //
    // 2. Migration path: `is_legacy_terminal_line_height` forced to always
    //    return `false`.
    //
    //   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:417:9:
    //   got 1.2, want 1
    //   test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 227 filtered out
    //
    //   (`migrates_the_old_terminal_line_height_default_to_preserve_tui_block_graphics`)
    //
    // 3. The workspace-keep-alive "rejects strings" asymmetry (this
    //    module's doc doesn't number this one, but it's the behaviour
    //    `normalize_workspace_keep_alive_minutes`'s own doc comment
    //    describes): made it parse a numeric string the way the two
    //    font-size normalizers do.
    //
    //   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:419:9:
    //   got 45, want 10
    //   test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 227 filtered out
    //
    //   (`workspace_keep_alive_minutes_falls_back_to_default_for_nan_missing_or_a_string`)
    //
    // 4. Theme registry migration: `normalize_theme`'s membership check
    //    short-circuited to always accept the value.
    //
    //   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:713:9:
    //   assertion `left == right` failed
    //     left: "terra"
    //    right: "crowbar"
    //   test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 227 filtered out
    //
    //   (`coerces_a_theme_id_the_registry_does_not_know_to_the_default_theme`)
    //
    // 5. This module's doc, item 1 finding, mutated to see if its own test
    //    would catch someone "fixing" it back to the schema default:
    //    `FILE_TREE_INDENT_SIZE_NON_FINITE_FALLBACK` 20.0 -> 16.0.
    //
    //   thread '...' panicked at crates/crowbar-core/src/settings/normalization.rs:417:9:
    //   got 16, want 20
    //   test result: FAILED. 0 passed; 1 failed; 0 ignored; 0 measured; 227 filtered out
    //
    //   (`file_tree_indent_size_non_finite_fallback_is_20_not_the_schema_default_of_16`)
    //
    // All five were reverted immediately after capture (`cp`/`sed -i` in
    // place, diffed against the pre-mutation copy, confirmed identical).
}
