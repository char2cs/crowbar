// Bundled UI font (local @font-face in theme-fonts.css). Chosen over Geist
// Variable after a live A/B in the running app; Geist Variable stays
// selectable in Settings. Does not touch DEFAULT_MONO_FONT_FAMILY below —
// terminal/code stays on Geist Mono Variable regardless of the UI font.
export const DEFAULT_UI_FONT_FAMILY = 'CalSansUI'
export const DEFAULT_MONO_FONT_FAMILY = 'Geist Mono Variable'
// Display face for headings (Markdown h1-h3, dialog titles, settings section
// titles, etc.) — independent of the UI font above, and user-configurable
// since some people find a display serif too much for every heading.
export const DEFAULT_HEADING_FONT_FAMILY = 'Instrument Serif'
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
