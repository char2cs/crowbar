import { DEFAULT_MARKDOWN_FONT_SIZE } from '@/features/settings/config/typography-defaults'

// Base px size of the markdown document surface, apart from Monaco's code `fontSize` (reading vs
// code size). markdown-editor.css is all `em`, so this rescales the document; whole px suffice.
export const MARKDOWN_FONT_SIZE_MIN = 12
export const MARKDOWN_FONT_SIZE_MAX = 24
export const MARKDOWN_FONT_SIZE_STEP = 1
export const MARKDOWN_FONT_SIZE_DEFAULT = DEFAULT_MARKDOWN_FONT_SIZE

export function normalizeMarkdownFontSize(value: unknown): number {
  const parsed = typeof value === 'number' ? value : typeof value === 'string' ? Number(value) : NaN
  if (!Number.isFinite(parsed)) return MARKDOWN_FONT_SIZE_DEFAULT

  const snapped = Math.round(parsed / MARKDOWN_FONT_SIZE_STEP) * MARKDOWN_FONT_SIZE_STEP
  return Math.min(MARKDOWN_FONT_SIZE_MAX, Math.max(MARKDOWN_FONT_SIZE_MIN, snapped))
}
