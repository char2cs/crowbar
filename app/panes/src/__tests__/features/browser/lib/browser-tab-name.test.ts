import { describe, expect, it } from 'vitest'
import { browserTabName } from '@/features/browser/lib/browser-tab-name'

describe('browserTabName', () => {
  it('prefers the page title', () => {
    expect(browserTabName('Example', 'https://example.com/x')).toBe('Example')
  })

  it('falls back to the host', () => {
    expect(browserTabName(undefined, 'https://example.com/x')).toBe('example.com')
  })

  it('treats a blank title as no title', () => {
    expect(browserTabName('   ', 'https://example.com/x')).toBe('example.com')
  })

  it('is "Browser" for an empty tab', () => {
    expect(browserTabName(undefined, '')).toBe('Browser')
  })

  it('is the raw url when it does not parse', () => {
    expect(browserTabName('', 'not a url')).toBe('not a url')
  })
})
