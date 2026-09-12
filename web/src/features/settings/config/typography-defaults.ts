// Bundled UI font (loaded via @font-face in theme.css). Kept as the default so
// the app's typography is driven by Settings without requiring a new webfont.
export const DEFAULT_UI_FONT_FAMILY = 'CalSansUI'
export const DEFAULT_MONO_FONT_FAMILY = 'Geist Mono Variable'
// The terminal renders through xterm's DOM renderer (see resolve-font.ts), so
// there is no longer any reason to prefer a static cut — that preference only
// existed to keep the WebGL glyph atlas alive. Sharing the editor's variable cut
// keeps typography consistent across editor and terminal.
//
// Font choice is load-bearing under the DOM renderer: box-drawing glyphs come
// from whichever font in the fallback chain supplies them, and a mismatch
// between that font's advance width and the primary font's produces visible
// seams in TUI borders. Geist Mono Variable bundles its own box-drawing/block
// glyphs (U+2500-259F — JetBrains Mono Variable had none), so it no longer
// depends on a fallback for them at all; for whatever else still falls
// through, its advance width measures identical to JetBrains Mono Variable's
// (canvas measureText, 64px: 38.40px for both) and within 0.34% of the macOS
// fallback (Menlo, 38.53px), so the seams stay sub-pixel exactly as before.
export const DEFAULT_TERMINAL_FONT_FAMILY = 'Geist Mono Variable'

export const DEFAULT_CODE_FONT_SIZE = 14
export const DEFAULT_UI_FONT_SIZE_OFFSET = 1
export const DEFAULT_UI_FONT_SIZE = DEFAULT_CODE_FONT_SIZE + DEFAULT_UI_FONT_SIZE_OFFSET
export const DEFAULT_TERMINAL_FONT_SIZE = DEFAULT_CODE_FONT_SIZE
// The markdown document surface reads at a document size, not a code size —
// deliberately larger than DEFAULT_CODE_FONT_SIZE and tracked separately.
// 16 is what the `1rem` that markdown-editor.css used to hardcode resolves to
// (nothing overrides the root font-size), so the default changes nothing.
export const DEFAULT_MARKDOWN_FONT_SIZE = 16
