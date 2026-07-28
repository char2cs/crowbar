// Bundled UI font (loaded via @font-face in theme.css). Kept as the default so
// the app's typography is driven by Settings without requiring a new webfont.
export const DEFAULT_UI_FONT_FAMILY = 'CalSansUI'
export const DEFAULT_MONO_FONT_FAMILY = 'JetBrains Mono Variable'
// The terminal renders through xterm's DOM renderer (see resolve-font.ts), so
// there is no longer any reason to prefer a static cut — that preference only
// existed to keep the WebGL glyph atlas alive. Sharing the editor's variable cut
// keeps typography consistent across editor and terminal.
//
// Font choice is load-bearing under the DOM renderer: box-drawing glyphs come
// from whichever font in the fallback chain supplies them, and a mismatch
// between that font's advance width and the primary font's produces visible
// seams in TUI borders. JetBrains Mono Variable sits within 0.35% of the macOS
// fallback (Menlo), so the seams stay sub-pixel.
export const DEFAULT_TERMINAL_FONT_FAMILY = 'JetBrains Mono Variable'

export const DEFAULT_CODE_FONT_SIZE = 14
export const DEFAULT_UI_FONT_SIZE_OFFSET = 1
export const DEFAULT_UI_FONT_SIZE = DEFAULT_CODE_FONT_SIZE + DEFAULT_UI_FONT_SIZE_OFFSET
export const DEFAULT_TERMINAL_FONT_SIZE = DEFAULT_CODE_FONT_SIZE
// The markdown document surface reads at a document size, not a code size —
// deliberately larger than DEFAULT_CODE_FONT_SIZE and tracked separately.
// 16 is what the `1rem` that markdown-editor.css used to hardcode resolves to
// (nothing overrides the root font-size), so the default changes nothing.
export const DEFAULT_MARKDOWN_FONT_SIZE = 16
