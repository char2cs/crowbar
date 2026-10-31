import { describe, expect, it } from 'vitest'
import { cloneGhostRows, cloneGhostTab } from '@/components/layout/drag-ghost'

function row(): HTMLElement {
  const el = document.createElement('div')
  el.className = 'border-transparent text-foreground hover:bg-sidebar-element-idle'
  el.textContent = 'a row'
  return el
}

// bg-background is the dark charcoal in dark mode, near-invisible over the dark
// sidebar; the control-surface utility pins the raised white in every mode.
describe('lifted drag ghost surface', () => {
  it('lifts a cloned row onto the control surface, not the ambient background', () => {
    const { nodes } = cloneGhostRows([row()], 1)
    expect(nodes[0].classList.contains('control-surface')).toBe(true)
    expect(nodes[0].classList.contains('bg-background')).toBe(false)
  })

  it('strips the open-row ground and inverted text an active row wears, on the row and its children', () => {
    const el = row()
    el.classList.add('bg-sidebar-element-idle', 'text-foreground-inverse')
    const glyph = document.createElement('span')
    glyph.className = 'size-4 text-foreground-inverse'
    el.appendChild(glyph)
    const { nodes } = cloneGhostRows([el], 1)
    expect(nodes[0].classList.contains('bg-sidebar-element-idle')).toBe(false)
    expect(nodes[0].classList.contains('text-foreground-inverse')).toBe(false)
    expect(nodes[0].querySelector('.text-foreground-inverse')).toBeNull()
    expect(el.classList.contains('text-foreground-inverse')).toBe(true)
  })

  it('lifts a cloned view tab onto the control surface too', () => {
    const { nodes } = cloneGhostTab(row())
    expect(nodes[0].classList.contains('control-surface')).toBe(true)
  })
})
