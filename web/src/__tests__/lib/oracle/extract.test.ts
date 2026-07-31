import { afterEach, describe, expect, it, vi } from 'vitest'
import type { OracleState } from '@/lib/oracle/extract'
import {
  extractSnapshot,
  extractSnapshotSource,
  oracleDetectTheme,
  oracleFirstFontFamily,
  oracleFontWeight,
  oracleContentSized,
  oracleLineSized,
  oracleIsClipped,
  oracleMeasureNormalLineHeight,
  oracleNormalizeColor,
  oracleNormalizeState,
  oracleRelativeBounds,
  oracleResolveLineHeight,
} from '@/lib/oracle/extract'

// jsdom has no layout engine, so every geometry test below stubs the rects it
// needs. That is the point: what is under test is the *arithmetic and
// normalisation* the contract specifies, not the browser's box model.

// ── colour normalisation (ANCHORS.md §3: `#rrggbbaa`, sRGB) ──────────────────

describe('oracleNormalizeColor', () => {
  it('normalises the legacy rgb()/rgba() forms getComputedStyle returns', () => {
    expect(oracleNormalizeColor('rgb(200, 204, 212)')).toBe('#c8ccd4ff')
    expect(oracleNormalizeColor('rgba(200, 204, 212, 1)')).toBe('#c8ccd4ff')
    expect(oracleNormalizeColor('rgba(255, 0, 0, 0.5)')).toBe('#ff000080')
  })

  it('normalises the modern slash-alpha and percentage forms', () => {
    expect(oracleNormalizeColor('rgb(200 204 212)')).toBe('#c8ccd4ff')
    expect(oracleNormalizeColor('rgb(200 204 212 / 0.5)')).toBe('#c8ccd480')
    expect(oracleNormalizeColor('rgb(100% 0% 0% / 50%)')).toBe('#ff000080')
  })

  it('treats every flavour of nothing as fully transparent black', () => {
    expect(oracleNormalizeColor('transparent')).toBe('#00000000')
    expect(oracleNormalizeColor('rgba(0, 0, 0, 0)')).toBe('#00000000')
    expect(oracleNormalizeColor('')).toBe('#00000000')
    expect(oracleNormalizeColor(null)).toBe('#00000000')
    expect(oracleNormalizeColor(undefined)).toBe('#00000000')
  })

  it('always emits eight hex digits, never six (v1.1)', () => {
    // Six digits is rejected at load: the only alpha an extractor could invent
    // is `ff`, which makes "opaque" and "forgot the alpha" indistinguishable.
    for (const input of [
      'rgb(200, 204, 212)',
      '#fff',
      '#c8ccd4',
      'oklch(1 0 0)',
      'color(srgb 1 0 0)',
      'hsl(0, 100%, 50%)',
      'transparent',
      'not-a-colour-at-all',
    ]) {
      expect(oracleNormalizeColor(input)).toMatch(/^#[0-9a-f]{8}$/)
    }
  })

  it('normalises hex in all four lengths', () => {
    expect(oracleNormalizeColor('#fff')).toBe('#ffffffff')
    expect(oracleNormalizeColor('#f008')).toBe('#ff000088')
    expect(oracleNormalizeColor('#c8ccd4')).toBe('#c8ccd4ff')
    expect(oracleNormalizeColor('#c8ccd480')).toBe('#c8ccd480')
  })

  it('normalises color(srgb …) — what color-mix() resolves to', () => {
    expect(oracleNormalizeColor('color(srgb 1 0 0)')).toBe('#ff0000ff')
    expect(oracleNormalizeColor('color(srgb 0 0 0 / 0.4)')).toBe('#00000066')
  })

  it('converts oklch() — the form this app authors its theme tokens in', () => {
    // oklch(1 0 0) is pure white and oklch(0 0 0) pure black by definition; both
    // pin the transfer function's ends without depending on a gamut fixture.
    expect(oracleNormalizeColor('oklch(1 0 0)')).toBe('#ffffffff')
    expect(oracleNormalizeColor('oklch(0 0 0)')).toBe('#000000ff')
    expect(oracleNormalizeColor('oklch(1 0 0 / 60%)')).toBe('#ffffff99')
  })

  it('converts hsl()', () => {
    expect(oracleNormalizeColor('hsl(0, 100%, 50%)')).toBe('#ff0000ff')
    expect(oracleNormalizeColor('hsl(120 100% 50% / 0.5)')).toBe('#00ff0080')
  })
})

// ── font (ANCHORS.md §3) ─────────────────────────────────────────────────────

describe('oracleFontWeight', () => {
  it('maps the CSS keywords onto the numeric scale the contract requires', () => {
    expect(oracleFontWeight('normal')).toBe(400)
    expect(oracleFontWeight('bold')).toBe(700)
    expect(oracleFontWeight('lighter')).toBe(100)
    expect(oracleFontWeight('bolder')).toBe(700)
  })

  it('passes numerics through and falls back to 400 on nonsense', () => {
    expect(oracleFontWeight('500')).toBe(500)
    expect(oracleFontWeight(600)).toBe(600)
    expect(oracleFontWeight('')).toBe(400)
    expect(oracleFontWeight('inherit')).toBe(400)
  })

  it('does not clamp to 100–900 — v1.1 accepts the full CSS 1–1000 range', () => {
    expect(oracleFontWeight('1')).toBe(1)
    expect(oracleFontWeight('850')).toBe(850)
    expect(oracleFontWeight('1000')).toBe(1000)
  })
})

// ── state vocabulary (ANCHORS.md v1.1) ───────────────────────────────────────

describe('oracleNormalizeState', () => {
  it('fills every missing key from the document, never inventing a synonym', () => {
    expect(oracleNormalizeState(undefined, 320.4, 'dark')).toEqual({
      width: 320,
      theme: 'dark',
      content: 'normal',
      flags: [],
    })
  })

  it('rounds width to an integer', () => {
    expect(oracleNormalizeState({ width: 319.6 }, 0, 'light').width).toBe(320)
  })

  it('treats flags as a set: deduplicated and sorted', () => {
    // ["selected","hover"] and ["hover","selected"] are the same matrix cell;
    // refusing to compare them would be a false alarm.
    expect(
      oracleNormalizeState({ flags: ['selected', 'hover', 'hover'] }, 0, 'light').flags,
    ).toEqual(['hover', 'selected'])
  })

  // The union types reject these at compile time, which is the first line of
  // defence. These casts exercise the second: a value arriving from an injected
  // script, where there is no type checker at all.
  const untyped = (state: Record<string, unknown>) => state as Partial<OracleState>

  it('lowercases the caller’s casing rather than emitting a refusal', () => {
    const state = oracleNormalizeState(
      untyped({ theme: 'Dark', content: 'Overflow', flags: ['Hover'] }),
      0,
      'light',
    )
    expect(state).toEqual({ width: 0, theme: 'dark', content: 'overflow', flags: ['hover'] })
  })

  it('throws on a value outside the vocabulary instead of shipping it', () => {
    // The differ answers an unknown value by refusing the comparison, which
    // surfaces three steps later as "0 deltas". Fail where the string was typed.
    expect(() => oracleNormalizeState(untyped({ theme: 'crowbar' }), 0, 'light')).toThrow(
      /state\.theme/,
    )
    expect(() => oracleNormalizeState(untyped({ content: 'truncated' }), 0, 'light')).toThrow(
      /state\.content/,
    )
    expect(() => oracleNormalizeState(untyped({ flags: ['pressed'] }), 0, 'light')).toThrow(
      /state\.flags/,
    )
  })
})

describe('oracleDetectTheme', () => {
  afterEach(() => {
    document.documentElement.className = ''
    document.documentElement.removeAttribute('data-theme')
    vi.restoreAllMocks()
  })

  it('reads the `dark` class first', () => {
    document.documentElement.className = 'dark'
    expect(oracleDetectTheme(document)).toBe('dark')
  })

  it('reads data-theme when it happens to be one of the two words', () => {
    document.documentElement.setAttribute('data-theme', 'Light')
    expect(oracleDetectTheme(document)).toBe('light')
  })

  it('falls back to luminance for a *named* theme like this app’s', () => {
    // `data-theme="crowbar"` is neither word. Guessing `light` because the name
    // was unfamiliar would refuse every comparison in the run.
    document.documentElement.setAttribute('data-theme', 'crowbar')
    vi.spyOn(window, 'getComputedStyle').mockImplementation(
      (() =>
        ({
          colorScheme: 'normal',
          backgroundColor: 'rgb(20, 20, 20)',
        }) as unknown as CSSStyleDeclaration) as typeof window.getComputedStyle,
    )
    expect(oracleDetectTheme(document)).toBe('dark')
  })
})

describe('oracleFirstFontFamily', () => {
  it('resolves to the first family, unquoted', () => {
    expect(oracleFirstFontFamily('"Inter", system-ui, sans-serif')).toBe('Inter')
    expect(oracleFirstFontFamily("'JetBrains Mono', monospace")).toBe('JetBrains Mono')
    expect(oracleFirstFontFamily('sans-serif')).toBe('sans-serif')
    expect(oracleFirstFontFamily('')).toBe('')
  })
})

describe('oracleResolveLineHeight', () => {
  it('takes px straight', () => {
    expect(oracleResolveLineHeight('17.55px', 13, 0)).toBeCloseTo(17.55, 5)
  })

  it('resolves `normal` to the measured used value, never the string', () => {
    expect(oracleResolveLineHeight('normal', 13, 17.55)).toBeCloseTo(17.55, 5)
  })

  it('falls back to 1.2em only when `normal` could not be measured', () => {
    expect(oracleResolveLineHeight('normal', 13, 0)).toBeCloseTo(15.6, 5)
  })

  it('resolves unitless multipliers and percentages against the font size', () => {
    expect(oracleResolveLineHeight('1.35', 20, 0)).toBeCloseTo(27, 5)
    expect(oracleResolveLineHeight('150%', 20, 0)).toBeCloseTo(30, 5)
  })
})

describe('oracleMeasureNormalLineHeight', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('lays a probe out off-screen, reads its height, and leaves the DOM clean', () => {
    vi.spyOn(HTMLElement.prototype, 'getBoundingClientRect').mockReturnValue({
      x: 0,
      y: 0,
      left: 0,
      top: 0,
      right: 0,
      bottom: 17.55,
      width: 0,
      height: 17.55,
      toJSON: () => ({}),
    } as DOMRect)

    const host = document.createElement('span')
    document.body.appendChild(host)
    const before = document.body.childNodes.length

    const measured = oracleMeasureNormalLineHeight(host, {
      fontFamily: 'Inter',
      fontSize: '13px',
      fontWeight: '400',
      fontStyle: 'normal',
    })

    expect(measured).toBeCloseTo(17.55, 5)
    expect(document.body.childNodes.length).toBe(before)
    host.remove()
  })
})

// ── bounds (ANCHORS.md §4) ───────────────────────────────────────────────────

describe('oracleRelativeBounds', () => {
  const root = { left: 100, top: 50, width: 320, height: 24 }

  it('puts the root itself at exactly {0,0}', () => {
    expect(oracleRelativeBounds(root, root)).toEqual({ x: 0, y: 0, w: 320, h: 24 })
  })

  it('cancels the window origin out of every other anchor', () => {
    expect(oracleRelativeBounds({ left: 130, top: 54, width: 118, height: 16 }, root)).toEqual({
      x: 30,
      y: 4,
      w: 118,
      h: 16,
    })
  })

  it('handles a root to the right of and below the anchor', () => {
    expect(oracleRelativeBounds({ left: 90, top: 40, width: 10, height: 10 }, root)).toEqual({
      x: -10,
      y: -10,
      w: 10,
      h: 10,
    })
  })
})

// ── clipping (ANCHORS.md §3) ─────────────────────────────────────────────────

describe('oracleIsClipped', () => {
  it('does not call a sub-pixel overhang a clip', () => {
    // scrollWidth/clientWidth are integers, so a 100.4px string in a 100px box
    // reads as a whole pixel of overflow that never paints an ellipsis.
    expect(
      oracleIsClipped({ scrollWidth: 101, clientWidth: 100, textWidth: 100.4, contentWidth: 100 }),
    ).toBe(false)
  })

  it('calls a real overhang a clip', () => {
    expect(
      oracleIsClipped({ scrollWidth: 187, clientWidth: 118, textWidth: 186.5, contentWidth: 118 }),
    ).toBe(true)
  })

  it('respects an explicit epsilon', () => {
    expect(
      oracleIsClipped({
        scrollWidth: 0,
        clientWidth: 0,
        textWidth: 100.4,
        contentWidth: 100,
        epsilon: 0.25,
      }),
    ).toBe(true)
  })

  it('falls back to scrollWidth vs clientWidth when there is no text to measure', () => {
    expect(oracleIsClipped({ scrollWidth: 187, clientWidth: 118 })).toBe(true)
    expect(oracleIsClipped({ scrollWidth: 118, clientWidth: 118 })).toBe(false)
    expect(oracleIsClipped({ scrollWidth: 187, clientWidth: 118, contentWidth: 0 })).toBe(true)
  })
})

// ── the v1.5 declaration ─────────────────────────────────────────────────────

describe('oracleContentSized', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  function el(markup: string): Element {
    document.body.innerHTML = markup
    return document.body.firstElementChild as Element
  }

  it('reads the three spellings an author can actually produce', () => {
    // React renders `data-x={true}` as `="true"`; hand-written markup gives `""`.
    expect(oracleContentSized(el('<span data-oracle-content-sized="true"></span>'))).toBe(true)
    expect(oracleContentSized(el('<span data-oracle-content-sized></span>'))).toBe(true)
    expect(oracleContentSized(el('<span data-oracle-content-sized="TRUE"></span>'))).toBe(true)
  })

  it('treats an absent attribute and an explicit false as the same fact', () => {
    expect(oracleContentSized(el('<span></span>'))).toBe(false)
    expect(oracleContentSized(el('<span data-oracle-content-sized="false"></span>'))).toBe(false)
    expect(oracleContentSized(el('<span data-oracle-content-sized=" False "></span>'))).toBe(false)
  })

  it('does not read the flag off the id attribute', () => {
    expect(oracleContentSized(el('<span data-oracle-id="git-row-badge"></span>'))).toBe(false)
  })
})

// ── the v1.6 declaration ─────────────────────────────────────────────────────

describe('oracleLineSized', () => {
  afterEach(() => {
    document.body.innerHTML = ''
  })

  function el(markup: string): Element {
    document.body.innerHTML = markup
    return document.body.firstElementChild as Element
  }

  it('reads the three spellings an author can actually produce', () => {
    expect(oracleLineSized(el('<span data-oracle-line-sized="true"></span>'))).toBe(true)
    expect(oracleLineSized(el('<span data-oracle-line-sized></span>'))).toBe(true)
    expect(oracleLineSized(el('<span data-oracle-line-sized="TRUE"></span>'))).toBe(true)
  })

  it('treats an absent attribute and an explicit false as the same fact', () => {
    expect(oracleLineSized(el('<span></span>'))).toBe(false)
    expect(oracleLineSized(el('<span data-oracle-line-sized="false"></span>'))).toBe(false)
    expect(oracleLineSized(el('<span data-oracle-line-sized=" False "></span>'))).toBe(false)
  })

  it('is a different claim from content_sized and neither implies the other', () => {
    // Two independent properties of the same box: `git-row-name` is line-sized
    // and not content-sized (it is the flexible sibling), the badge is
    // content-sized and not line-sized (`sm:h-4` pins its height).
    const line = el('<span data-oracle-line-sized="true"></span>')
    expect(oracleLineSized(line)).toBe(true)
    expect(oracleContentSized(line)).toBe(false)

    const content = el('<span data-oracle-content-sized="true"></span>')
    expect(oracleContentSized(content)).toBe(true)
    expect(oracleLineSized(content)).toBe(false)

    const both = el('<span data-oracle-content-sized data-oracle-line-sized></span>')
    expect(oracleContentSized(both)).toBe(true)
    expect(oracleLineSized(both)).toBe(true)
  })

  it('does not read the flag off the id attribute', () => {
    expect(oracleLineSized(el('<span data-oracle-id="git-row-name"></span>'))).toBe(false)
  })
})

// ── the walk itself, including a pseudo-backed anchor (ANCHORS.md §3) ────────

interface FakeStyle {
  [key: string]: string
}

const BASE_STYLE: FakeStyle = {
  display: 'block',
  visibility: 'visible',
  opacity: '1',
  overflowX: 'visible',
  overflowY: 'visible',
  content: 'normal',
  backgroundColor: 'transparent',
  color: 'rgb(0, 0, 0)',
  borderTopLeftRadius: '0px',
  borderTopWidth: '0px',
  borderRightWidth: '0px',
  borderBottomWidth: '0px',
  borderLeftWidth: '0px',
  borderTopColor: 'transparent',
  paddingLeft: '0px',
  paddingRight: '0px',
  fontFamily: '"Inter", sans-serif',
  fontSize: '13px',
  fontWeight: 'normal',
  fontStyle: 'normal',
  lineHeight: 'normal',
}

function stubRect(el: Element, box: { left: number; top: number; width: number; height: number }) {
  Object.defineProperty(el, 'getBoundingClientRect', {
    configurable: true,
    value: () => ({
      x: box.left,
      y: box.top,
      left: box.left,
      top: box.top,
      right: box.left + box.width,
      bottom: box.top + box.height,
      width: box.width,
      height: box.height,
      toJSON: () => ({}),
    }),
  })
}

/**
 * A gate-target-shaped row: a `.file-tree-item` container whose *only* visible
 * background is painted by `::before`, a transparent button, one indent guide
 * and a truncating filename.
 */
function mountRow() {
  document.body.innerHTML = `
    <div id="row" data-oracle-id="git-row-item">
      <span id="guide" data-oracle-id="git-row-guide-0"></span>
      <button id="btn" data-oracle-id="git-row-button">
        <span id="name" data-oracle-id="git-row-name" data-oracle-line-sized="true">resolve-terminal-connection.ts</span>
        <span id="badge" data-oracle-id="git-row-badge" data-oracle-content-sized="true">uncommitted</span>
      </button>
    </div>`

  const row = document.getElementById('row') as HTMLElement
  const guide = document.getElementById('guide') as HTMLElement
  const btn = document.getElementById('btn') as HTMLElement
  const name = document.getElementById('name') as HTMLElement
  const badge = document.getElementById('badge') as HTMLElement

  stubRect(row, { left: 100, top: 50, width: 320, height: 24 })
  stubRect(guide, { left: 110, top: 50, width: 7, height: 24 })
  stubRect(btn, { left: 100, top: 50, width: 320, height: 24 })
  stubRect(name, { left: 130, top: 54, width: 118, height: 16 })
  stubRect(badge, { left: 252, top: 54, width: 74.11, height: 16 })

  const styles = new Map<Element, FakeStyle>([
    // The button is pinned transparent in every state by file-explorer-tree.css;
    // the row's paint lives entirely on the pseudo below.
    [row, { backgroundColor: 'transparent', borderTopLeftRadius: '8px' }],
    [guide, { backgroundColor: 'transparent', opacity: '0.9' }],
    [
      btn,
      {
        display: 'flex',
        backgroundColor: 'transparent',
        borderTopLeftRadius: '2px',
        borderTopWidth: '1px',
        borderRightWidth: '1px',
        borderBottomWidth: '1px',
        borderLeftWidth: '1px',
        borderTopColor: 'transparent',
      },
    ],
    [
      name,
      {
        overflowX: 'hidden',
        overflowY: 'hidden',
        color: 'rgb(200, 204, 212)',
        lineHeight: '17.55px',
        fontWeight: '500',
      },
    ],
    [
      badge,
      {
        color: 'rgb(255, 185, 0)',
        backgroundColor: 'rgba(254, 154, 0, 0.16)',
        borderTopLeftRadius: '4px',
        borderTopWidth: '1px',
        borderTopColor: 'transparent',
        lineHeight: '13.33px',
        fontSize: '10px',
        fontWeight: '500',
      },
    ],
  ])

  const pseudoStyles = new Map<string, FakeStyle>([
    [
      'row::before',
      {
        content: '""',
        backgroundColor: 'rgba(255, 0, 0, 0.5)',
        borderTopLeftRadius: '2px',
        borderTopWidth: '0px',
        borderTopColor: 'transparent',
      },
    ],
  ])

  vi.spyOn(window, 'getComputedStyle').mockImplementation(((
    el: Element,
    pseudo?: string | null,
  ) => {
    if (pseudo) {
      const key = (el.id || '') + pseudo
      const found = pseudoStyles.get(key)
      return { ...BASE_STYLE, content: 'none', ...(found || {}) } as unknown as CSSStyleDeclaration
    }
    return { ...BASE_STYLE, ...(styles.get(el) || {}) } as unknown as CSSStyleDeclaration
  }) as typeof window.getComputedStyle)

  // Ranges report layout, and jsdom has none — hand back the advance width the
  // real engine would measure for the filename.
  vi.spyOn(document, 'createRange').mockImplementation(
    () =>
      ({
        selectNodeContents: () => {},
        getClientRects: () => [{ width: 186.5 }],
        getBoundingClientRect: () => ({ width: 186.5 }),
      }) as unknown as Range,
  )

  return { row, guide, btn, name, badge }
}

describe('extractSnapshot', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('emits a v1 snapshot rooted on the anchor it was told to root on', () => {
    mountRow()
    const snapshot = extractSnapshot({
      surface: 'git-status-row',
      root: 'git-row-item',
      state: { theme: 'dark', content: 'overflow', flags: ['hover'] },
    })

    expect(snapshot.schema).toBe(1)
    expect(snapshot.surface).toBe('git-status-row')
    expect(snapshot.root).toBe('git-row-item')
    expect(snapshot.state).toEqual({
      width: 320,
      theme: 'dark',
      content: 'overflow',
      flags: ['hover'],
    })
    expect(snapshot.anchors.map((a) => a.id)).toEqual([
      'git-row-item',
      'git-row-guide-0',
      'git-row-button',
      'git-row-name',
      'git-row-badge',
    ])
  })

  it('reports every anchor relative to the root, with the root at the origin', () => {
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })
    const byId = Object.fromEntries(anchors.map((a) => [a.id, a]))

    expect(byId['git-row-item'].bounds).toEqual({ x: 0, y: 0, w: 320, h: 24 })
    expect(byId['git-row-guide-0'].bounds).toEqual({ x: 10, y: 0, w: 7, h: 24 })
    expect(byId['git-row-name'].bounds).toEqual({ x: 30, y: 4, w: 118, h: 16 })
  })

  it('reads a pseudo-backed anchor off ::before, not off the element', () => {
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })
    const byId = Object.fromEntries(anchors.map((a) => [a.id, a]))

    // The element itself is transparent with an 8px radius; the pseudo paints
    // the row at 2px. Reading the element would report both wrongly.
    expect(byId['git-row-item'].bg).toBe('#ff000080')
    expect(byId['git-row-item'].radius).toBe(2)
    // The button really is transparent in every state — that is not a miss.
    expect(byId['git-row-button'].bg).toBe('#00000000')
    expect(byId['git-row-button'].border).toEqual({ w: 1, color: '#00000000' })
  })

  it('leaves an anchor unbacked when the host has no such pseudo', () => {
    mountRow()
    const { anchors } = extractSnapshot({
      surface: 'git-status-row',
      pseudo: { 'git-row-button': '::before' },
    })
    const byId = Object.fromEntries(anchors.map((a) => [a.id, a]))

    // `content: none` means the pseudo does not exist, so the element's own
    // paint stands — for the row that is its real 8px radius.
    expect(byId['git-row-item'].radius).toBe(8)
    expect(byId['git-row-button'].radius).toBe(2)
  })

  it('records full text, unclipped advance width, and the truncation', () => {
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })
    const name = anchors.find((a) => a.id === 'git-row-name')

    expect(name?.text).toBe('resolve-terminal-connection.ts')
    expect(name?.text_width).toBe(186.5)
    expect(name?.clipped).toBe(true)
    expect(name?.fg).toBe('#c8ccd4ff')
    expect(name?.font).toEqual({ size: 13, weight: 500, family: 'Inter', line_height: 17.55 })
  })

  it('omits the text fields on anchors that paint no text of their own', () => {
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })
    const button = anchors.find((a) => a.id === 'git-row-button')

    // The button's descendants carry the text; `textContent` would have made it
    // look like the button paints the whole row.
    expect(button?.text).toBeUndefined()
    expect(button?.font).toBeUndefined()
    expect(button?.visible).toBe(true)
  })

  it('puts the root first in `anchors`, at exactly the origin (v1.1 load error)', () => {
    mountRow()
    const snapshot = extractSnapshot({ surface: 'git-status-row', root: 'git-row-item' })

    expect(snapshot.anchors[0].id).toBe(snapshot.root)
    expect(snapshot.anchors[0].bounds.x).toBe(0)
    expect(snapshot.anchors[0].bounds.y).toBe(0)
  })

  it('emits only the fields §3 defines — an unknown one is a hard failure', () => {
    mountRow()
    const snapshot = extractSnapshot({ surface: 'git-status-row' })

    expect(Object.keys(snapshot).sort()).toEqual(['anchors', 'root', 'schema', 'state', 'surface'])
    expect(Object.keys(snapshot.state).sort()).toEqual(['content', 'flags', 'theme', 'width'])

    const known = [
      'bg',
      'bounds',
      'border',
      'clipped',
      'content_sized',
      'fg',
      'font',
      'id',
      'line_sized',
      'radius',
      'text',
      'text_width',
      'visible',
    ]
    for (const anchor of snapshot.anchors) {
      for (const key of Object.keys(anchor)) {
        expect(known).toContain(key)
      }
    }
  })

  it('emits the whole text group or none of it (a partial is a ranked delta)', () => {
    mountRow()
    const group = ['fg', 'text', 'text_width', 'clipped', 'font'] as const
    for (const anchor of extractSnapshot({ surface: 'git-status-row' }).anchors) {
      const present = group.filter((k) => anchor[k] !== undefined).length
      expect(present === 0 || present === group.length).toBe(true)
    }
  })

  it('emits content_sized only on the anchors that declare it (v1.5)', () => {
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })
    const byId = Object.fromEntries(anchors.map((a) => [a.id, a]))

    expect(byId['git-row-badge'].content_sized).toBe(true)
    // Absent, not `false`. v1.5 makes the missing key and an explicit `false`
    // the same fact, and the GPUI side omits it too — a key one extractor
    // writes and the other does not is a difference in the wire shape that
    // says nothing about the UI.
    for (const id of ['git-row-item', 'git-row-guide-0', 'git-row-button', 'git-row-name']) {
      expect(byId[id].content_sized).toBeUndefined()
      expect('content_sized' in byId[id]).toBe(false)
    }
  })

  it('emits line_sized only on the anchors that declare it (v1.6)', () => {
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })
    const byId = Object.fromEntries(anchors.map((a) => [a.id, a]))

    expect(byId['git-row-name'].line_sized).toBe(true)
    // The two flags are independent claims about the same box: the name is
    // line-sized and not content-sized, the badge the other way round.
    expect(byId['git-row-name'].content_sized).toBeUndefined()
    expect(byId['git-row-badge'].content_sized).toBe(true)
    expect(byId['git-row-badge'].line_sized).toBeUndefined()

    // Absent, not `false`, exactly as for content_sized.
    for (const id of ['git-row-item', 'git-row-guide-0', 'git-row-button', 'git-row-badge']) {
      expect('line_sized' in byId[id]).toBe(false)
    }
  })

  it('carries the line height a line_sized anchor will be compared against', () => {
    // The rule is `bounds.h` against `font.line_height`, so the declaration is
    // worthless without the font group beside it — and the differ refuses a
    // snapshot that declares one without the other, by anchor name.
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })

    for (const anchor of anchors) {
      if (anchor.line_sized) {
        expect(anchor.font).toBeDefined()
        expect(typeof anchor.font?.line_height).toBe('number')
      }
    }
  })

  it('throws rather than silently snapshotting the wrong subtree', () => {
    mountRow()
    expect(() => extractSnapshot({ surface: 'git-status-row', root: 'no-such-anchor' })).toThrow(
      /root anchor/,
    )
    expect(() => extractSnapshot({ surface: 'git-status-row', scope: '#nope' })).toThrow(/scope/)
  })
})

// ── injectability ────────────────────────────────────────────────────────────

describe('extractSnapshotSource', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  it('produces a self-contained IIFE whose result matches the module path', () => {
    mountRow()
    const options = { surface: 'git-status-row', root: 'git-row-item' }
    const source = extractSnapshotSource(options)

    // Indirect eval, so the source runs in global scope with nothing but the
    // DOM available — which is the whole claim this function makes.
    const emitted = JSON.parse((0, eval)(source) as string)

    expect(emitted).toEqual(JSON.parse(JSON.stringify(extractSnapshot(options))))
  })

  it('refuses an Element scope, which cannot survive serialisation into source', () => {
    mountRow()
    const row = document.getElementById('row') as HTMLElement
    expect(() => extractSnapshotSource({ surface: 'x', scope: row })).toThrow(/selector/)
  })
})
