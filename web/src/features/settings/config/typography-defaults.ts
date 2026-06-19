export const DEFAULT_UI_FONT_FAMILY = 'IBM Plex Sans Variable'
export const DEFAULT_MONO_FONT_FAMILY = 'JetBrains Mono Variable'
// Terminal uses the non-variable cut so xterm.js can use its WebGL renderer.
// Variable fonts trigger Canvas2D fallback (texture atlas misalignment), which
// causes 130-180ms main-thread stalls on full-screen TUI apps like htop.
export const DEFAULT_TERMINAL_FONT_FAMILY = 'JetBrains Mono'

export const DEFAULT_CODE_FONT_SIZE = 14
export const DEFAULT_UI_FONT_SIZE_OFFSET = 1
export const DEFAULT_UI_FONT_SIZE = DEFAULT_CODE_FONT_SIZE + DEFAULT_UI_FONT_SIZE_OFFSET
export const DEFAULT_TERMINAL_FONT_SIZE = DEFAULT_CODE_FONT_SIZE
