//! Font/size constants.
//!
//! Ported from `web/src/features/settings/config/typography-defaults.ts`,
//! verbatim — this file is pure data in the TS source too, no logic to
//! translate.

/// Bundled UI font (loaded via `@font-face` in `theme.css` on the web side;
/// the native port's equivalent asset-loading is Phase 3/`crowbar-ui`
/// territory). Kept as the default so the app's typography is driven by
/// Settings without requiring a new webfont.
pub const DEFAULT_UI_FONT_FAMILY: &str = "CalSansUI";
pub const DEFAULT_MONO_FONT_FAMILY: &str = "JetBrains Mono Variable";

/// The terminal shares the editor's variable cut for typographic
/// consistency. See the TS source's own comment for the box-drawing-glyph
/// sub-pixel-seam reasoning (a DOM/xterm rendering concern that does not
/// apply to this crate, kept here only because it explains *why* this
/// constant equals [`DEFAULT_MONO_FONT_FAMILY`] rather than being its own
/// independent choice).
pub const DEFAULT_TERMINAL_FONT_FAMILY: &str = "JetBrains Mono Variable";

pub const DEFAULT_CODE_FONT_SIZE: f64 = 14.0;
pub const DEFAULT_UI_FONT_SIZE_OFFSET: f64 = 1.0;
pub const DEFAULT_UI_FONT_SIZE: f64 = DEFAULT_CODE_FONT_SIZE + DEFAULT_UI_FONT_SIZE_OFFSET;
pub const DEFAULT_TERMINAL_FONT_SIZE: f64 = DEFAULT_CODE_FONT_SIZE;

/// The markdown document surface reads at a document size, not a code size
/// — deliberately larger than [`DEFAULT_CODE_FONT_SIZE`] and tracked
/// separately. 16 is what the TS source's now-removed `1rem` CSS default
/// resolved to; the default changes nothing for an existing user.
pub const DEFAULT_MARKDOWN_FONT_SIZE: f64 = 16.0;
