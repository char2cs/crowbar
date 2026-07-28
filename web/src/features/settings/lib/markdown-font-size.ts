import { DEFAULT_MARKDOWN_FONT_SIZE } from '@/features/settings/config/typography-defaults'

/**
 * Base type size for the Plate markdown document surface
 * (`.crowbar-markdown-editor`), in pixels.
 *
 * Separate from the code `fontSize`: that one drives Monaco, including the
 * Source view of the very same markdown file, and a reading size and a code
 * size are not the same want. Every other size in markdown-editor.css is
 * expressed in `em`, so this single value rescales the whole document.
 *
 * Whole pixels, unlike `uiFontSize`'s 0.5px steps — this scales one surface
 * rather than the entire chrome, so sub-pixel tuning buys nothing.
 */
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
