import { describe, expect, it } from 'vitest'
import { createStripDrag } from '@/components/sidebar/lib/tab-strip-drag'

const BOX = { left: 100, right: 500, top: 0, bottom: 32 }
const SLOTS = [0, 1, 2, 3].map((i) => ({ left: 100 + i * 100, width: 100 }))

describe('createStripDrag', () => {
  function build() {
    const strip = document.createElement('div')
    const rect = (r: Partial<DOMRect>) => () => ({ ...r, toJSON: () => r }) as DOMRect
    strip.getBoundingClientRect = rect({ ...BOX, width: 400, height: 32 })
    const tabs = SLOTS.map((s) => {
      const tab = document.createElement('div')
      const row = document.createElement('div')
      row.getBoundingClientRect = rect({ left: s.left, width: 100, top: 0, height: 32 })
      tab.getBoundingClientRect = rect({ left: s.left, width: 100, top: 0, height: 32 })
      tab.appendChild(row)
      strip.appendChild(tab)
      return { tab, row }
    })
    return { strip, tabs }
  }

  it('slides the passed tabs and hides the source slot while the pointer is in the band', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(420, 16, null)
    expect(tabs.map((t) => t.tab.style.transform)).toEqual([
      '',
      'translateX(-100px)',
      'translateX(-100px)',
      '',
    ])
    expect(tabs[0].tab.style.visibility).toBe('hidden')
    expect(drag.drop(420)).toEqual({ el: tabs[3].tab, mode: 'before' })
  })

  it('pins the ghost into the strip in the band and frees it outside', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(520, 20, ghost)
    expect(ghost.style.transform).toBe('translate3d(400px, 0px, 0)')
    drag.move(900, 200, ghost)
    expect(ghost.style.transform).toBe('translate3d(850px, 184px, 0)')
    expect(tabs.map((t) => t.tab.style.transform)).toEqual(['', '', '', ''])
    expect(tabs[0].tab.style.visibility).toBe('')
  })

  it('turns back into the in-strip tab when the pointer returns to the band', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(320, 200, null)
    drag.move(320, 16, null)
    expect(tabs[1].tab.style.transform).toBe('translateX(-100px)')
  })

  it('gives every tab back its place on dispose', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(320, 16, null)
    drag.dispose()
    expect(tabs.map((t) => t.tab.style.transform + t.tab.style.visibility)).toEqual([
      '',
      '',
      '',
      '',
    ])
  })

  const slides = (tabs: { tab: HTMLElement }[]) => tabs.map((t) => t.tab.style.transform)

  it('is in the band within a strip height of the strip, and out past it', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    expect(drag.inBand(300, 63)).toBe(true)
    expect(drag.inBand(300, -31)).toBe(true)
    expect(drag.inBand(531, 16)).toBe(true)
    expect(drag.inBand(300, 65)).toBe(false)
    expect(drag.inBand(300, -33)).toBe(false)
    expect(drag.inBand(40, 16)).toBe(false)
  })

  it('keeps the ghost under the grab point and stops it at both ends of the strip', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[0].row, { dx: 30, dy: 16 })!
    drag.move(250, 16, ghost)
    expect(ghost.style.transform).toBe('translate3d(220px, 0px, 0)')
    drag.move(90, 16, ghost)
    expect(ghost.style.transform).toBe('translate3d(100px, 0px, 0)')
    drag.move(520, 16, ghost)
    expect(ghost.style.transform).toBe('translate3d(400px, 0px, 0)')
  })

  it('slides the tabs passed over toward the vacated slot, in both directions', () => {
    const { strip, tabs } = build()
    const fwd = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    fwd.move(420, 16, null)
    expect(slides(tabs)).toEqual(['', 'translateX(-100px)', 'translateX(-100px)', ''])
    fwd.dispose()
    const back = createStripDrag(strip, tabs[3].row, { dx: 50, dy: 16 })!
    back.move(180, 16, null)
    expect(slides(tabs)).toEqual(['', 'translateX(100px)', 'translateX(100px)', ''])
  })

  it('drops before the tab now at the target index, after the last at the end, and nowhere when unmoved', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    expect(drag.drop(150)).toBeNull()
    expect(drag.drop(320)).toEqual({ el: tabs[2].tab, mode: 'before' })
    expect(drag.drop(520)).toEqual({ el: tabs[3].tab, mode: 'after' })
    const last = createStripDrag(strip, tabs[3].row, { dx: 50, dy: 16 })!
    expect(last.drop(120)).toEqual({ el: tabs[0].tab, mode: 'before' })
  })
})
