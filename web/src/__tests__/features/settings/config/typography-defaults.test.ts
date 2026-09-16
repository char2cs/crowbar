import { describe, expect, it } from 'vitest'
import {
  DEFAULT_MONO_FONT_FAMILY,
  DEFAULT_TERMINAL_FONT_FAMILY,
} from '@/features/settings/config/typography-defaults'

describe('typography defaults', () => {
  it('bundles Geist Mono Variable as the default editor font', () => {
    expect(DEFAULT_MONO_FONT_FAMILY).toBe('Geist Mono Variable')
  })

  it('bundles Geist Mono Variable as the default terminal font', () => {
    expect(DEFAULT_TERMINAL_FONT_FAMILY).toBe('Geist Mono Variable')
  })

  it('never falls back to the dropped JetBrains Mono default', () => {
    expect(DEFAULT_MONO_FONT_FAMILY).not.toMatch(/jetbrains/i)
    expect(DEFAULT_TERMINAL_FONT_FAMILY).not.toMatch(/jetbrains/i)
  })
})
