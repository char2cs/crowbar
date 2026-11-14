import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { domAgentBody, type DomOp, type PointerOp } from '@/features/browser/lib/dom-agent'
import type { PickResult } from '@/features/browser/lib/pointer'

const AsyncFunction = Object.getPrototypeOf(async () => {}).constructor as new (
  body: string,
) => () => Promise<unknown>

function run<T = unknown>(op: DomOp | PointerOp, args: Record<string, unknown> = {}): Promise<T> {
  return new AsyncFunction(domAgentBody(op, args))() as Promise<T>
}

type Rect = [number, number, number, number]
const rects = new WeakMap<Element, Rect>()
function place(el: Element, rect: Rect): Element {
  rects.set(el, rect)
  return el
}
const $ = (selector: string) => document.querySelector(selector) as HTMLElement

beforeEach(() => {
  document.body.innerHTML = ''
  document.title = 'Test page'
  vi.spyOn(Element.prototype, 'getBoundingClientRect').mockImplementation(function (this: Element) {
    const [x, y, w, h] = rects.get(this) ?? [0, 0, 0, 0]
    return { x, y, left: x, top: y, width: w, height: h, right: x + w, bottom: y + h } as DOMRect
  })
})

afterEach(async () => {
  await run('cancelPick')
  vi.restoreAllMocks()
})

function mouse(type: string, target: EventTarget, x: number, y: number, init: MouseEventInit = {}) {
  target.dispatchEvent(
    new MouseEvent(type, { bubbles: true, cancelable: true, clientX: x, clientY: y, ...init }),
  )
}
function click(target: Element, x = 1, y = 1, init: MouseEventInit = {}) {
  mouse('mousedown', target, x, y, init)
  mouse('mouseup', target, x, y, init)
  mouse('click', target, x, y, init)
}
const overlay = () => document.querySelector('[data-crowbar-pick]')

describe('pick: element', () => {
  it('captures the clicked element as a compact item without firing the page handlers', async () => {
    document.body.innerHTML = `
      <h1>Checkout</h1>
      <section class="summary"><h2>Order summary</h2><button id="pay">Pay now</button></section>`
    const pay = place($('#pay'), [640, 300, 80, 30]) as HTMLElement
    const onPage = vi.fn()
    document.addEventListener('click', onPage)
    document.addEventListener('mousedown', onPage)

    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    expect(overlay()).not.toBeNull()
    click(pay)
    const result = await pending

    expect(onPage.mock.calls.map((c) => c[0].type)).toEqual([])
    expect(overlay()).toBeNull()
    expect(result).toMatchObject({
      kind: 'element',
      title: 'Test page',
      more: 0,
      box: [640, 300, 80, 30],
      items: [
        {
          s: '#pay',
          tag: 'button',
          role: 'button',
          name: 'Pay now',
          text: 'Pay now',
          r: [640, 300, 80, 30],
          h: 'Order summary',
          i: 0,
        },
      ],
    })
    document.removeEventListener('click', onPage)
    document.removeEventListener('mousedown', onPage)
  })

  it('cancels on Escape and leaves nothing behind', async () => {
    document.body.innerHTML = '<button>One</button>'
    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }))
    expect(await pending).toBeNull()
    expect(overlay()).toBeNull()

    const onPage = vi.fn()
    document.addEventListener('click', onPage)
    click($('button'))
    expect(onPage).toHaveBeenCalled()
    document.removeEventListener('click', onPage)
  })

  it('ends with null when the page is torn down mid-pick', async () => {
    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    window.dispatchEvent(new Event('pagehide'))
    expect(await pending).toBeNull()
    expect(overlay()).toBeNull()
  })

  it('cancelPick resolves the pending pick and reports whether one was active', async () => {
    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    expect(await run('cancelPick')).toBe(true)
    expect(await pending).toBeNull()
    expect(await run('cancelPick')).toBe(false)
  })
})

describe('pick: multi', () => {
  it('collects with Shift+click, removes a repeat, and finishes on Enter', async () => {
    document.body.innerHTML = '<p id="a">A</p><p id="b">B</p><p id="c">C</p>'
    place($('#a'), [0, 0, 10, 10])
    place($('#b'), [20, 0, 10, 10])
    place($('#c'), [40, 0, 10, 10])
    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    click($('#a'), 1, 1, { shiftKey: true })
    click($('#b'), 21, 1, { shiftKey: true })
    click($('#c'), 41, 1, { shiftKey: true })
    click($('#b'), 21, 1, { shiftKey: true })
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    const result = await pending

    expect(result?.kind).toBe('multi')
    expect(result?.items.map((i) => i.s)).toEqual(['#a', '#c'])
    expect(result?.box).toEqual([0, 0, 50, 10])
  })

  it('caps items at 10 and counts the rest in more', async () => {
    document.body.innerHTML = Array.from({ length: 13 }, (_, n) => `<i id="n${n}">${n}</i>`).join(
      '',
    )
    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    for (let n = 0; n < 13; n++) {
      click(place($(`#n${n}`), [n * 10, 0, 8, 8]) as HTMLElement, n * 10, 1, { shiftKey: true })
    }
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }))
    const result = await pending
    expect(result?.items).toHaveLength(10)
    expect(result?.more).toBe(3)
  })
})

describe('pick: region', () => {
  const table = () => {
    document.body.innerHTML = `
      <table id="t"><thead><tr><th>Item</th><th>Price</th></tr></thead>
      <tbody id="body">
        <tr><td>Pro plan</td><td>$29</td></tr>
        <tr><td>Seats</td><td>$2</td></tr>
        <tr><td>Total</td><td>$31</td></tr>
      </tbody></table><p id="outside">Footer</p>`
    place($('#t'), [0, 0, 200, 190])
    place($('thead'), [0, 0, 200, 100])
    place($('#body'), [0, 100, 200, 90])
    $('#body')
      .querySelectorAll('tr')
      .forEach((row, n) => {
        place(row, [0, 100 + n * 30, 200, 30])
        row.querySelectorAll('td').forEach((td, c) => place(td, [c * 100, 100 + n * 30, 100, 30]))
      })
    place($('#outside'), [0, 300, 200, 20])
  }

  async function drag(from: [number, number], to: [number, number]) {
    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    mouse('mousedown', document.body, ...from)
    mouse('mousemove', document.body, ...to)
    expect(overlay()?.children[2]).toHaveProperty('style.display', 'block')
    mouse('mouseup', document.body, ...to)
    return pending
  }

  it('keeps the outermost fully-enclosed element instead of every cell', async () => {
    table()
    const result = await drag([0, 98], [210, 195])
    expect(result?.kind).toBe('region')
    expect(result?.items.map((i) => i.tag)).toEqual(['tbody'])
    expect(result?.box).toEqual([0, 98, 210, 97])
  })

  it('applies the 60% rule: a mostly-covered row is in, a half-covered one is out', async () => {
    table()
    const mostly = await drag([0, 100], [200, 150])
    expect(mostly?.items.map((i) => i.text)).toEqual(['Pro plan$29', 'Seats$2'])
    const half = await drag([0, 100], [200, 145])
    expect(half?.items.map((i) => i.text)).toEqual(['Pro plan$29'])
  })

  it('collapses a crowd of siblings into their parent when it fills the box', async () => {
    document.body.innerHTML = `<ul id="list">${Array.from({ length: 14 }, (_, n) => `<li>Row ${n}</li>`).join('')}</ul>`
    place($('#list'), [0, 0, 100, 140])
    document.querySelectorAll('li').forEach((li, n) => place(li, [0, n * 10, 100, 10]))
    const result = await drag([0, 0], [100, 140])
    expect(result?.items.map((i) => i.tag)).toEqual(['ul'])
  })
})

describe('pick: text', () => {
  it('reports the selected text with its container', async () => {
    document.body.innerHTML = '<p id="p">Hello brave new world</p>'
    place($('#p'), [10, 10, 200, 20])
    const range = document.createRange()
    range.selectNodeContents($('#p'))
    const selection = window.getSelection()!
    selection.removeAllRanges()
    selection.addRange(range)

    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    mouse('mousedown', $('#p'), 12, 12, { altKey: true })
    mouse('mouseup', $('#p'), 80, 12, { altKey: true })
    const result = await pending

    expect(result).toMatchObject({ kind: 'text', quote: 'Hello brave new world' })
    expect(result?.items[0]).toMatchObject({ s: '#p', tag: 'p' })
  })
})

describe('selectors', () => {
  async function selectorOf(el: Element): Promise<string> {
    const pending = run<PickResult | null>('pick')
    await Promise.resolve()
    click(place(el, [1, 1, 5, 5]) as HTMLElement)
    return (await pending)!.items[0].s
  }

  it('prefers id, then data-testid, then aria-label', async () => {
    document.body.innerHTML = `
      <div data-testid="card"><span>x</span></div>
      <button aria-label="Close dialog">x</button>`
    expect(await selectorOf($('[data-testid]'))).toBe('[data-testid="card"]')
    expect(await selectorOf($('button'))).toBe('button[aria-label="Close dialog"]')
  })

  it('builds a short unique class and nth-of-type chain, ignoring generated classes', async () => {
    document.body.innerHTML = `
      <main><section class="summary css-1a2b3c"><p>one</p><p>two</p></section>
      <section class="other"><p>three</p><p>four</p></section></main>`
    const target = document.querySelectorAll('p')[1]
    const selector = await selectorOf(target)
    expect(selector).toBe('section.summary:nth-of-type(1) > p:nth-of-type(2)')
    expect(document.querySelector(selector)).toBe(target)
  })

  it('stays within 120 characters and still finds the element', async () => {
    const deep = Array.from({ length: 30 }, () => '<div class="layer-wrapper-segment">').join('')
    document.body.innerHTML = `${deep}<b>deep</b>${'</div>'.repeat(30)}`
    const selector = await selectorOf($('b'))
    expect(selector.length).toBeLessThanOrEqual(120)
    expect(document.querySelector(selector)).toBe($('b'))
  })
})

describe('resolve', () => {
  const item = (over: Record<string, unknown>) => ({
    s: '',
    tag: 'button',
    text: '',
    h: '',
    i: 0,
    ...over,
  })

  it('finds an item by selector and hands out a ref the click op can use', async () => {
    document.body.innerHTML = '<button id="go">Go</button>'
    const onClick = vi.fn()
    $('#go').addEventListener('click', onClick)
    const out = await run<{ items: { found: boolean; ref: string; matchedBy: string }[] }>(
      'resolve',
      { items: [item({ s: '#go', text: 'Go' })], detail: 'refs' },
    )
    expect(out.items[0]).toMatchObject({ found: true, matchedBy: 'selector' })
    expect(out.items[0].ref).toMatch(/^e\d+$/)

    await run('click', { ref: out.items[0].ref })
    expect(onClick).toHaveBeenCalledTimes(1)
  })

  it('falls back to the fingerprint after the selector stops matching', async () => {
    document.body.innerHTML = '<h2>Plans</h2><div><button class="old">Upgrade</button></div>'
    const out = await run<{ items: { found: boolean; matchedBy: string }[] }>('resolve', {
      items: [item({ s: 'button.old.gone', text: 'Upgrade', h: 'Plans' })],
    })
    expect(out.items[0]).toMatchObject({ found: true, matchedBy: 'fingerprint' })
  })

  it('does not accept a selector hit whose text changed, and reports a miss', async () => {
    document.body.innerHTML = '<button id="go">Something else</button>'
    const out = await run<{ items: { found: boolean }[] }>('resolve', {
      items: [item({ s: '#go', text: 'Go' }), item({ s: '#nope', text: 'Absent' })],
    })
    expect(out.items).toEqual([{ found: false }, { found: false }])
  })

  it('returns text, styles and html on request, truncating long html', async () => {
    document.body.innerHTML = `<div id="big" style="color: red">${'x'.repeat(3000)}</div>`
    const args = { items: [item({ s: '#big', tag: 'div', text: 'x'.repeat(80) })] }
    const read = async (detail: string) =>
      (await run<{ items: Record<string, unknown>[] }>('resolve', { ...args, detail })).items[0]

    expect((await read('text')).text).toHaveLength(1000)
    expect((await read('styles')).styles).toMatchObject({ color: 'rgb(255, 0, 0)' })
    const html = await read('html')
    expect(String(html.html)).toHaveLength(2000)
    expect(html.truncated).toBe(true)
  })
})
