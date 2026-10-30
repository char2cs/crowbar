import { describe, expect, it } from 'vitest'
import { createStripDrag } from '@/components/sidebar/lib/tab-strip-drag'

const BOX = { left: 100, right: 500, top: 0, bottom: 32 }
const SLOTS = [0, 1, 2, 3].map((i) => ({ left: 100 + i * 100, width: 100 }))

describe('createStripDrag', () => {
  function build(stripRight = BOX.right, pinnedCount = 0) {
    const strip = document.createElement('div')
    const rect = (r: Partial<DOMRect>) => () => ({ ...r, toJSON: () => r }) as DOMRect
    strip.getBoundingClientRect = rect({
      ...BOX,
      right: stripRight,
      width: stripRight - BOX.left,
      height: 32,
    })
    const tabs = SLOTS.map((s) => {
      const tab = document.createElement('div')
      const row = document.createElement('div')
      row.getBoundingClientRect = rect({ left: s.left, width: 100, top: 0, height: 32 })
      tab.getBoundingClientRect = rect({ left: s.left, width: 100, top: 0, height: 32 })
      tab.appendChild(row)
      if (strip.children.length < pinnedCount) tab.setAttribute('data-pinned', '')
      strip.appendChild(tab)
      return { tab, row }
    })
    return { strip, tabs }
  }

  it('slides the passed tabs and hides the source slot while the pointer is in the band', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(400, 16, null)
    expect(tabs.map((t) => t.tab.style.transform)).toEqual([
      '',
      'translateX(-100px)',
      'translateX(-100px)',
      '',
    ])
    expect(tabs[0].tab.style.visibility).toBe('hidden')
    expect(drag.drop(400)).toEqual({ el: tabs[3].tab, mode: 'before' })
  })

  it('pins the ghost into the strip in the band and frees it outside', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(500, 20, ghost)
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
    drag.move(100, 16, ghost)
    expect(ghost.style.transform).toBe('translate3d(100px, 0px, 0)')
    drag.move(500, 16, ghost)
    expect(ghost.style.transform).toBe('translate3d(400px, 0px, 0)')
  })

  it('slides the tabs passed over toward the vacated slot, in both directions', () => {
    const { strip, tabs } = build()
    const fwd = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    fwd.move(400, 16, null)
    expect(slides(tabs)).toEqual(['', 'translateX(-100px)', 'translateX(-100px)', ''])
    fwd.dispose()
    const back = createStripDrag(strip, tabs[3].row, { dx: 50, dy: 16 })!
    back.move(200, 16, null)
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

  const ghostAt = (ghost: HTMLElement) => {
    const m = /translate3d\((-?[\d.]+)px, (-?[\d.]+)px/.exec(ghost.style.transform)!
    return { x: +m[1], y: +m[2] }
  }

  it('never puts a transition on the ghost, so it cannot trail the pointer', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[2].row, { dx: 50, dy: 16 })!
    drag.move(260, 16, ghost)
    drag.move(300, 90, ghost)
    drag.move(300, 16, ghost)
    expect(ghost.style.transition).toBe('')
  })

  it('moves the ghost continuously across the band edge, never jumping by the margin', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[2].row, { dx: 50, dy: 16 })!
    drag.move(300, 16, ghost)
    let prev = ghostAt(ghost)
    for (let y = 18; y <= 120; y += 2) {
      drag.move(300, y, ghost)
      const g = ghostAt(ghost)
      expect(Math.abs(g.y - prev.y)).toBeLessThanOrEqual(6)
      expect(Math.abs(g.x - prev.x)).toBeLessThanOrEqual(0.01)
      prev = g
    }
    expect(prev).toEqual({ x: 250, y: 104 })
  })

  it('follows the pointer exactly while it is inside the strip', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[2].row, { dx: 50, dy: 16 })!
    for (const x of [301, 307, 330, 331, 380]) {
      drag.move(x, 14, ghost)
      expect(ghostAt(ghost)).toEqual({ x: x - 50, y: 0 })
    }
  })

  it('reaches the first and the last slot from the middle', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[2].row, { dx: 50, dy: 16 })!
    expect(drag.drop(60)).toEqual({ el: tabs[0].tab, mode: 'before' })
    expect(drag.drop(560)).toEqual({ el: tabs[3].tab, mode: 'after' })
  })

  it('keeps the slide steady while the pointer jitters across a slot boundary', () => {
    const { strip, tabs } = build()
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(300, 16, null)
    const settled = tabs.map((t) => t.tab.style.transform)
    for (const x of [304, 297, 303, 298, 302]) {
      drag.move(x, 16, null)
      expect(tabs.map((t) => t.tab.style.transform)).toEqual(settled)
    }
  })

  it('settles the dropped tab from where the ghost was, and keeps that through dispose', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(400, 16, ghost)
    drag.settle(400)
    drag.dispose()
    // The ghost sat at 350; the tab lands in slot 2, which starts at 300.
    expect(tabs[0].tab.style.transform).toBe('translate(50px, 0px)')
    expect(tabs[0].tab.style.transition).toBe('none')
  })

  it('stops at the last tab, not the strip end, when the tabs do not fill the strip', () => {
    const { strip, tabs } = build(700)
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
    drag.move(650, 16, ghost)
    expect(ghostAt(ghost).x).toBe(400)
    expect(drag.drop(650)).toEqual({ el: tabs[3].tab, mode: 'after' })
  })

  it('ghosts a group tab as the whole tab: every member, the close button, no stray test ids', () => {
    const { strip, tabs } = build()
    const shell = document.createElement('div')
    shell.setAttribute('data-testid', 'recents-set-v1')
    const first = tabs[1].row
    first.setAttribute('role', 'treeitem')
    const second = document.createElement('div')
    second.setAttribute('role', 'treeitem')
    second.setAttribute('data-testid', 'recents-row-b')
    const close = document.createElement('button')
    close.setAttribute('aria-label', 'Close view')
    tabs[1].tab.appendChild(shell)
    shell.append(first, second, close)
    const drag = createStripDrag(strip, first, { dx: 50, dy: 16 })!
    const rows = drag.ghostRows()
    expect(rows.count).toBe(1)
    expect(rows.nodes).toHaveLength(1)
    expect(rows.nodes[0].querySelectorAll('[role=treeitem]')).toHaveLength(2)
    expect(rows.nodes[0].querySelector('[aria-label="Close view"]')).not.toBeNull()
    expect(rows.nodes[0].querySelector('[data-testid]')).toBeNull()
  })

  it('puts the first paint exactly where the first move would, so pickup does not jump', () => {
    const { strip, tabs } = build()
    const ghost = document.createElement('div')
    const drag = createStripDrag(strip, document.createElement('div'), { dx: 3, dy: 3 })!
    const start = drag.position(300, 50)
    drag.move(300, 50, ghost)
    expect(ghostAt(ghost)).toEqual(start)
    expect(start.y).toBeGreaterThan(0)
    expect(start.y).toBeLessThan(34)
    expect(tabs).toHaveLength(4)
  })

  describe('a subject from outside the strip (a pane dragged up)', () => {
    const outside = () => document.createElement('div')

    it('opens a gap at the insertion index and takes no source slot', () => {
      const { strip, tabs } = build()
      const drag = createStripDrag(strip, outside(), { dx: 3, dy: 3 })!
      drag.move(200, 16, null)
      expect(slides(tabs)).toEqual([
        '',
        'translateX(100px)',
        'translateX(100px)',
        'translateX(100px)',
      ])
      expect(tabs.every((t) => t.tab.style.visibility === '')).toBe(true)
    })

    it('reaches before the first tab and after the last', () => {
      const { strip, tabs } = build()
      const drag = createStripDrag(strip, outside(), { dx: 3, dy: 3 })!
      expect(drag.drop(-40)).toEqual({ el: tabs[0].tab, mode: 'before' })
      expect(drag.drop(200)).toEqual({ el: tabs[1].tab, mode: 'before' })
      expect(drag.drop(900)).toEqual({ el: tabs[3].tab, mode: 'after' })
    })

    it('is a tab-sized ghost held at its centre, pinned to the strip row', () => {
      const { strip, tabs } = build()
      const ghost = document.createElement('div')
      const drag = createStripDrag(strip, outside(), { dx: 3, dy: 3 })!
      expect(drag.ghostRows()).toMatchObject({ width: 100, height: 32, count: 1, nodes: [] })
      drag.move(300, 20, ghost)
      expect(ghostAt(ghost)).toEqual({ x: 250, y: 0 })
      drag.move(300, 200, ghost)
      expect(ghostAt(ghost)).toEqual({ x: 250, y: 184 })
      expect(slides(tabs)).toEqual(['', '', '', ''])
    })
  })

  describe('with pinned tabs at the front', () => {
    it('keeps a pinned tab inside the pinned group, however far it is pulled', () => {
      const { strip, tabs } = build(BOX.right, 2)
      const ghost = document.createElement('div')
      const drag = createStripDrag(strip, tabs[0].row, { dx: 50, dy: 16 })!
      drag.move(480, 16, ghost)
      expect(ghostAt(ghost).x).toBe(200)
      expect(drag.drop(480)).toEqual({ el: tabs[2].tab, mode: 'before' })
    })

    it('keeps an unpinned tab behind the pinned group', () => {
      const { strip, tabs } = build(BOX.right, 2)
      const ghost = document.createElement('div')
      const drag = createStripDrag(strip, tabs[3].row, { dx: 50, dy: 16 })!
      drag.move(110, 16, ghost)
      expect(ghostAt(ghost).x).toBe(300)
      expect(drag.drop(110)).toEqual({ el: tabs[2].tab, mode: 'before' })
    })

    it('inserts an outside subject behind the pinned group, never among it', () => {
      const { strip, tabs } = build(BOX.right, 2)
      const drag = createStripDrag(strip, document.createElement('div'), { dx: 3, dy: 3 })!
      drag.move(110, 16, null)
      expect(slides(tabs)).toEqual(['', '', 'translateX(100px)', 'translateX(100px)'])
      expect(drag.drop(110)).toEqual({ el: tabs[2].tab, mode: 'before' })
    })
  })
})
