import { describe, expect, it } from 'vitest'
import { normalizeBrowserUrl } from '@/features/browser/lib/normalize-url'
import { searchTemplateFor } from '@/features/browser/lib/search-engines'

const DDG = searchTemplateFor('duckduckgo', '')

describe('normalizeBrowserUrl', () => {
  it('prefixes bare hosts with https', () => {
    expect(normalizeBrowserUrl('example.com', DDG)).toBe('https://example.com')
    expect(normalizeBrowserUrl('docs.rs/tauri?x=1', DDG)).toBe('https://docs.rs/tauri?x=1')
  })

  it('keeps an explicit http or https url as typed', () => {
    expect(normalizeBrowserUrl('http://example.com/a?b=1', DDG)).toBe('http://example.com/a?b=1')
    expect(normalizeBrowserUrl('HTTPS://example.com', DDG)).toBe('HTTPS://example.com')
  })

  it('trims surrounding whitespace', () => {
    expect(normalizeBrowserUrl('  example.com  ', DDG)).toBe('https://example.com')
  })

  it('returns null for empty input', () => {
    expect(normalizeBrowserUrl('   ', DDG)).toBeNull()
  })

  it('treats a local dev host as http, where https would never connect', () => {
    expect(normalizeBrowserUrl('localhost:3000', DDG)).toBe('http://localhost:3000')
    expect(normalizeBrowserUrl('127.0.0.1:8080/x', DDG)).toBe('http://127.0.0.1:8080/x')
  })

  it('searches for text that is not a host, encoding the query', () => {
    expect(normalizeBrowserUrl('best pizza & pasta', DDG)).toBe(
      'https://duckduckgo.com/?q=best%20pizza%20%26%20pasta',
    )
    expect(normalizeBrowserUrl('cats', DDG)).toBe('https://duckduckgo.com/?q=cats')
  })

  it('never hands a non-web scheme to the webview, it searches for it', () => {
    expect(normalizeBrowserUrl('javascript:alert(1)', DDG)).toBe(
      'https://duckduckgo.com/?q=javascript%3Aalert(1)',
    )
  })
})

describe('searchTemplateFor', () => {
  it('maps each named engine to its own search url', () => {
    expect(searchTemplateFor('google', '')).toContain('google.com/search?q=%s')
    expect(searchTemplateFor('bing', '')).toContain('bing.com/search?q=%s')
  })

  it('uses a custom template that is an http(s) url with %s', () => {
    expect(searchTemplateFor('custom', 'https://kagi.com/search?q=%s')).toBe(
      'https://kagi.com/search?q=%s',
    )
  })

  it('falls back to the default engine for an unusable custom template', () => {
    expect(searchTemplateFor('custom', 'https://kagi.com/search')).toBe(DDG)
    expect(searchTemplateFor('custom', 'javascript:%s')).toBe(DDG)
  })
})
