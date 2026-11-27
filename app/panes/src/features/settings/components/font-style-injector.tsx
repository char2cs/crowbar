import { useEffect } from 'react'
import {
  DEFAULT_HEADING_FONT_FAMILY,
  DEFAULT_MONO_FONT_FAMILY,
  DEFAULT_UI_FONT_FAMILY,
} from '@/features/settings/config/typography-defaults'
import { useSettingsStore } from '@/features/settings/store'
import { currentPlatform, IS_WINDOWS } from '@/utils/platform'
import { normalizeMarkdownFontSize } from '../lib/markdown-font-size'
import { getUiFontScale, normalizeUiFontSize } from '../lib/ui-font-size'

const DEFAULT_MONO_FALLBACK =
  '"Geist Mono Variable", ui-monospace, SFMono-Regular, "SF Mono", Menlo, Consolas, "Liberation Mono", monospace'

const WINDOWS_MONO_FALLBACK =
  '"Geist Mono Variable", Consolas, "Cascadia Mono", "Cascadia Code", "Courier New", ui-monospace, monospace'

const DEFAULT_SANS_FALLBACK =
  'ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, "Helvetica Neue", Arial, sans-serif'

const WINDOWS_SANS_FALLBACK =
  '"Segoe UI", system-ui, -apple-system, BlinkMacSystemFont, Roboto, "Helvetica Neue", Arial, sans-serif'

const HEADING_FALLBACK = 'Georgia, "Times New Roman", serif'

function stripWrappingQuotes(value: string): string {
  const trimmed = value.trim()
  return trimmed.replace(/^['"]+|['"]+$/g, '')
}

function buildFontVariable(primary: string, fallback: string): string {
  const normalized = stripWrappingQuotes(primary)
  if (!normalized) return fallback

  if (normalized.includes(',')) {
    return `${normalized}, ${fallback}`
  }

  return `"${normalized}", ${fallback}`
}

export const FontStyleInjector = () => {
  const settings = useSettingsStore((s) => s.settings)

  useEffect(() => {
    document.documentElement.setAttribute('data-platform', currentPlatform)

    const requestedEditorFont =
      stripWrappingQuotes(settings.fontFamily || '') || DEFAULT_MONO_FONT_FAMILY
    const requestedUiFont =
      stripWrappingQuotes(settings.uiFontFamily || '') || DEFAULT_UI_FONT_FAMILY
    const requestedHeadingFont =
      stripWrappingQuotes(settings.headingFontFamily || '') || DEFAULT_HEADING_FONT_FAMILY

    const monoFallback = IS_WINDOWS ? WINDOWS_MONO_FALLBACK : DEFAULT_MONO_FALLBACK
    const sansFallback = IS_WINDOWS ? WINDOWS_SANS_FALLBACK : DEFAULT_SANS_FALLBACK

    document.documentElement.style.setProperty(
      '--editor-font-family',
      buildFontVariable(requestedEditorFont, monoFallback),
    )
    document.documentElement.style.setProperty(
      '--app-font-family',
      buildFontVariable(requestedUiFont, sansFallback),
    )
    document.documentElement.style.setProperty(
      '--app-heading-font-family',
      buildFontVariable(requestedHeadingFont, HEADING_FALLBACK),
    )

    const normalizedUiFontSize = normalizeUiFontSize(settings.uiFontSize)
    document.documentElement.style.setProperty('--app-ui-font-size', `${normalizedUiFontSize}px`)
    document.documentElement.style.setProperty(
      '--app-ui-scale',
      `${getUiFontScale(normalizedUiFontSize)}`,
    )

    document.documentElement.style.setProperty(
      '--md-base-font-size',
      `${normalizeMarkdownFontSize(settings.markdownFontSize)}px`,
    )
  }, [
    settings.fontFamily,
    settings.uiFontFamily,
    settings.uiFontSize,
    settings.markdownFontSize,
    settings.headingFontFamily,
  ])

  return null
}
