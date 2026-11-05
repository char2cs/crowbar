// Bundled UI font (local @font-face in theme-fonts.css); Geist Variable stays selectable.
// Terminal/code keep DEFAULT_MONO_FONT_FAMILY regardless of the UI font.
export const DEFAULT_UI_FONT_FAMILY = 'CalSansUI'
export const DEFAULT_MONO_FONT_FAMILY = 'Geist Mono Variable'
// Display face for headings (Markdown h1-h3, dialog titles, settings section
// titles, etc.) — independent of the UI font above, and user-configurable
// since some people find a display serif too much for every heading.
export const DEFAULT_HEADING_FONT_FAMILY = 'Instrument Serif'
// Load-bearing under xterm's DOM renderer: a fallback font with a different advance width seams
// TUI borders. Geist Mono Variable bundles its own box-drawing glyphs (U+2500-259F), and its
// advance width is within 0.34% of Menlo for whatever still falls through.
export const DEFAULT_TERMINAL_FONT_FAMILY = 'Geist Mono Variable'

export const DEFAULT_CODE_FONT_SIZE = 14
export const DEFAULT_UI_FONT_SIZE_OFFSET = 1
export const DEFAULT_UI_FONT_SIZE = DEFAULT_CODE_FONT_SIZE + DEFAULT_UI_FONT_SIZE_OFFSET
export const DEFAULT_TERMINAL_FONT_SIZE = DEFAULT_CODE_FONT_SIZE
// Markdown reads at a document size, not a code size, so it is tracked apart from
// DEFAULT_CODE_FONT_SIZE. 16 equals the root `1rem`.
export const DEFAULT_MARKDOWN_FONT_SIZE = 16
