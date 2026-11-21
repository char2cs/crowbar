import { describe, expect, it } from 'vitest'
import { paintPaneSurface } from '@/entries/paint-pane-surface'

describe('paintPaneSurface', () => {
  it.each([
    ['core.editor', 'var(--pane-background)'],
    ['core.review', 'var(--pane-background)'],
    ['core.chat', 'var(--pane-chrome-bg)'],
    ['core.console', 'var(--pane-chrome-bg)'],
  ])('fills a %s page with the surface develop paints under it', (typeId, colour) => {
    paintPaneSurface(document.body, typeId)
    expect(document.body.style.backgroundColor).toBe(colour)
  })
})
