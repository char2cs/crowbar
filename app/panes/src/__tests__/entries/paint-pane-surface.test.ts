import { describe, expect, it } from 'vitest'
import { paintPaneSurface } from '@/entries/paint-pane-surface'

describe('paintPaneSurface', () => {
  it.each([
    ['linux', 'core.editor', 'var(--pane-background)'],
    ['linux', 'core.review', 'var(--pane-background)'],
    ['linux', 'core.chat', 'var(--pane-chrome-bg)'],
    ['linux', 'core.console', 'var(--pane-chrome-bg)'],
    ['macos', 'core.editor', 'var(--pane-background)'],
    ['macos', 'core.chat', 'transparent'],
  ] as const)(
    'fills a %s %s page with the surface develop paints under it',
    (os, typeId, colour) => {
      paintPaneSurface(document.body, typeId, os)
      expect(document.body.style.backgroundColor).toBe(colour)
    },
  )
})
