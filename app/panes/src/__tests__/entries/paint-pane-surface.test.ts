import { describe, expect, it } from 'vitest'
import { paintPaneSurface } from '@/entries/paint-pane-surface'

describe('paintPaneSurface', () => {
  it('fills the body with the pane surface colour', () => {
    paintPaneSurface(document.body)
    expect(document.body.style.backgroundColor).toBe('var(--pane-chrome-bg)')
  })
})
