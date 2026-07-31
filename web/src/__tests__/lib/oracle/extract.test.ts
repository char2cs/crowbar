import { afterEach, describe, expect, it, vi } from 'vitest'
import type { OracleSnapshot, OracleState } from '@/lib/oracle/extract'
import {
  extractSnapshot,
  extractSnapshotSource,
  oracleDetectTheme,
  oracleFirstFontFamily,
  oracleFontWeight,
  oracleContentSized,
  oracleGeneratesBox,
  oracleLineSized,
  oracleIsClipped,
  oracleOverflowHidesText,
  oracleMeasureNormalLineHeight,
  oracleNormalizeColor,
  oracleNormalizeState,
  oracleRelativeBounds,
  oracleResolveLineHeight,
  oracleSelectDeclaredAnchors,
  oracleSurfaceScope,
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
    // The document is `dark` here because `Dark` now has to *agree* with it.
    // Casing is the only thing under test; pairing it with a light document
    // would be exercising the contradiction guard below instead.
    const state = oracleNormalizeState(
      untyped({ theme: 'Dark', content: 'Overflow', flags: ['Hover'] }),
      0,
      'dark',
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

  // ── the declared cell must match the document it is measuring ──────────────
  //
  // The defect: a reference was captured with `state: { theme: 'dark' }` while
  // a page reload had silently put the app back into **light**. The snapshot
  // came out *labelled* dark carrying light-theme values, and the oracle
  // reported a colour delta on every anchor whose real cause was the label.

  it('throws when the declared theme contradicts the document', () => {
    expect(() => oracleNormalizeState({ theme: 'dark' }, 320, 'light')).toThrow(
      /declares "dark".*document being measured is "light"/s,
    )
  })

  it('throws in the other direction too — it compares, it does not blacklist', () => {
    // A guard written as `if (t === 'dark') throw` would pass the test above.
    // This is the one that says the two values are actually being compared.
    expect(() => oracleNormalizeState({ theme: 'light' }, 320, 'dark')).toThrow(
      /declares "light".*document being measured is "dark"/s,
    )
  })

  it('catches the contradiction through the caller’s casing, not around it', () => {
    // `'Dark'` is lowercased before the comparison, so a mixed-case
    // declaration cannot slip past by failing a `===` against `'dark'`.
    expect(() => oracleNormalizeState(untyped({ theme: 'Dark' }), 320, 'light')).toThrow(
      /document being measured/,
    )
  })

  it('names both values and the way out, because the error is the whole signal', () => {
    // It is thrown inside an `execute_js` bridge: the message is all the
    // operator gets, so it has to say what was declared, what the document
    // actually is, and what to do about it.
    let message = ''
    try {
      oracleNormalizeState({ theme: 'dark' }, 320, 'light')
    } catch (err) {
      message = String(err)
    }
    expect(message).toContain('"dark"')
    expect(message).toContain('"light"')
    expect(message).toMatch(/no override/i)
  })

  it('lets an agreeing declaration through untouched (the control)', () => {
    // The check must cost a correct caller nothing — including the redundant
    // but legitimate habit of declaring the cell you drove the app into.
    expect(oracleNormalizeState({ theme: 'dark' }, 320, 'dark')).toEqual({
      width: 320,
      theme: 'dark',
      content: 'normal',
      flags: [],
    })
    expect(oracleNormalizeState({ theme: 'light' }, 320, 'light').theme).toBe('light')
  })

  it('still derives the theme when the caller declares none', () => {
    // Declaring nothing is not a contradiction — the guard must not fire on the
    // path that had no label to get wrong in the first place.
    expect(oracleNormalizeState({ width: 600 }, 320, 'dark').theme).toBe('dark')
    expect(oracleNormalizeState({ width: 600 }, 320, 'light').theme).toBe('light')
  })

  // `width` is NOT compared to the document — it is the *viewport* width and
  // `derivedWidth` is the root anchor's, a different quantity (the archived
  // `ref-600.json` declares 600 around a 294px root). What is enforced is that
  // a declared width is a number at all.

  it('does not compare a declared width against the root anchor’s width', () => {
    // The load-bearing case: `ref-600.json` is an honest capture. Rejecting it
    // would be a false alarm that blocks the viewport axis of the matrix.
    expect(oracleNormalizeState({ width: 600 }, 294, 'dark').width).toBe(600)
  })

  it('throws on a declared width that is not a number, rather than emitting 0', () => {
    // `Math.round(NaN)` used to land on `0`, so `width: '600px'` shipped a
    // snapshot labelled with a cell no run ever produced — the same silent
    // mislabel as the theme case.
    expect(() => oracleNormalizeState(untyped({ width: '600px' }), 294, 'dark')).toThrow(
      /state\.width/,
    )
    expect(() => oracleNormalizeState(untyped({ width: NaN }), 294, 'dark')).toThrow(/state\.width/)
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
      oracleIsClipped({
        scrollWidth: 101,
        clientWidth: 100,
        textWidth: 100.4,
        contentWidth: 100,
        overflowHidden: true,
      }),
    ).toBe(false)
  })

  it('calls a real overhang a clip', () => {
    expect(
      oracleIsClipped({
        scrollWidth: 187,
        clientWidth: 118,
        textWidth: 186.5,
        contentWidth: 118,
        overflowHidden: true,
      }),
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
        overflowHidden: true,
      }),
    ).toBe(true)
  })

  it('falls back to scrollWidth vs clientWidth when there is no text to measure', () => {
    expect(oracleIsClipped({ scrollWidth: 187, clientWidth: 118, overflowHidden: true })).toBe(true)
    expect(oracleIsClipped({ scrollWidth: 118, clientWidth: 118, overflowHidden: true })).toBe(
      false,
    )
    expect(
      oracleIsClipped({
        scrollWidth: 187,
        clientWidth: 118,
        contentWidth: 0,
        overflowHidden: true,
      }),
    ).toBe(true)
  })

  // ANCHORS.md v1.12. Overflowing the content box and being *truncated* are two
  // different facts, and v1.3 ruling 1 conflated them: the fractional test is
  // true for any overflowing text, hidden or not. §3 asks whether the text is
  // "visually truncated", and a glyph painted in full is not.
  it('is not a clip when nothing hides the overflow, however far it overhangs (v1.12)', () => {
    // The exact numbers P3.10 measured on the callout emoji, which the v1.3
    // predicate called truncated and the port — correctly — did not.
    expect(
      oracleIsClipped({
        scrollWidth: 22,
        clientWidth: 22,
        textWidth: 19,
        contentWidth: 14,
        overflowHidden: false,
      }),
    ).toBe(false)
    // And the same for the gate row's own numbers: 68px of overhang is still
    // not a truncation if the box paints all of it.
    expect(
      oracleIsClipped({
        scrollWidth: 187,
        clientWidth: 118,
        textWidth: 186.5,
        contentWidth: 118,
        overflowHidden: false,
      }),
    ).toBe(false)
  })

  it('gates the scrollWidth fallback on overflow too — it is the same question', () => {
    expect(oracleIsClipped({ scrollWidth: 187, clientWidth: 118, overflowHidden: false })).toBe(
      false,
    )
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
 *
 * `theme` puts the *document* into that theme, which is what
 * `oracleDetectTheme` reads. It is a parameter rather than a constant because a
 * declared `state.theme` is now checked against it: a test that wants to
 * snapshot a dark cell has to mount a dark document, exactly as a capture does.
 */
function mountRow(theme: 'light' | 'dark' = 'light') {
  document.documentElement.className = theme
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

/**
 * The tree the v1.7 opacity gap is about, in its smallest form:
 *
 * ```
 * <html> → <body> → #outer → #row[anchor] → #gap → #child[anchor]
 * ```
 *
 * `#outer` is the ancestor **above** the root anchor — `NavStack`'s `opacity-0`
 * layer — and `#gap` the unanchored ancestor **between** two anchors. Either can
 * be given a style, and either can be turned into an anchor, which is what
 * separates a comparable capture from one that is not.
 */
function mountProbeTree(
  styles: Record<string, FakeStyle>,
  anchored?: { outer?: boolean; gap?: boolean },
) {
  document.documentElement.className = 'light'
  const outerAttr = anchored?.outer ? ' data-oracle-id="probe-outer"' : ''
  const gapAttr = anchored?.gap ? ' data-oracle-id="probe-gap"' : ''
  document.body.innerHTML = `
    <div id="outer"${outerAttr}>
      <div id="row" data-oracle-id="probe-root">
        <div id="gap"${gapAttr}>
          <span id="child" data-oracle-id="probe-child"></span>
        </div>
      </div>
    </div>`

  const byId: Record<string, Element> = {
    html: document.documentElement,
    body: document.body,
    outer: document.getElementById('outer') as HTMLElement,
    row: document.getElementById('row') as HTMLElement,
    gap: document.getElementById('gap') as HTMLElement,
    child: document.getElementById('child') as HTMLElement,
  }
  stubRect(byId.outer, { left: 0, top: 0, width: 400, height: 40 })
  stubRect(byId.row, { left: 10, top: 10, width: 300, height: 20 })
  stubRect(byId.gap, { left: 10, top: 10, width: 300, height: 20 })
  stubRect(byId.child, { left: 20, top: 12, width: 60, height: 16 })

  const resolved = new Map<Element, FakeStyle>()
  for (const key of Object.keys(styles)) resolved.set(byId[key], styles[key])

  vi.spyOn(window, 'getComputedStyle').mockImplementation(((
    el: Element,
    pseudo?: string | null,
  ) => {
    if (pseudo) return { ...BASE_STYLE, content: 'none' } as unknown as CSSStyleDeclaration
    return { ...BASE_STYLE, ...(resolved.get(el) || {}) } as unknown as CSSStyleDeclaration
  }) as typeof window.getComputedStyle)

  return byId
}

/**
 * The probe tree's own surface name.
 *
 * It used to say `sidebar-carousel`, which was decorative — the tree carries
 * `probe-*` anchors and is rooted on `probe-root`. Since P2.11 a declared
 * surface owns a root and an anchor set, so that label now refuses itself:
 * `oracleSurfaceScope('sidebar-carousel')` pins `carousel-scrollport` and five
 * `carousel-*` anchors, none of which this tree has. That refusal is the
 * feature — a capture labelled with a surface it is not is exactly what the
 * declaration exists to stop — so the probe takes a name of its own.
 */
const PROBE = { surface: 'oracle-probe', root: 'probe-root' }

// ── what actually hides overflow (ANCHORS.md v1.12) ──────────────────────────

describe('oracleOverflowHidesText', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  /**
   * `#outer → #host → #el`, with a style per id and a rect per id. `#el` is the
   * element under test; either wrapper can be given a hiding `overflow`.
   */
  function mount(
    styles: Record<string, FakeStyle>,
    outerRect?: { left: number; top: number; width: number; height: number },
  ) {
    document.body.innerHTML = `
      <div id="outer"><div id="host"><span id="el">x</span></div></div>`
    const byId: Record<string, Element> = {
      outer: document.getElementById('outer') as HTMLElement,
      host: document.getElementById('host') as HTMLElement,
      el: document.getElementById('el') as HTMLElement,
    }
    stubRect(byId.outer, outerRect || { left: 0, top: 0, width: 400, height: 40 })
    stubRect(byId.host, { left: 0, top: 0, width: 400, height: 40 })
    stubRect(byId.el, { left: 10, top: 8, width: 24, height: 24 })

    const resolved = new Map<Element, FakeStyle>()
    for (const key of Object.keys(styles)) resolved.set(byId[key], styles[key])
    vi.spyOn(window, 'getComputedStyle').mockImplementation(((el: Element) => {
      return { ...BASE_STYLE, ...(resolved.get(el) || {}) } as unknown as CSSStyleDeclaration
    }) as typeof window.getComputedStyle)

    return byId
  }

  const EL_BOX = { left: 10, top: 8, width: 24, height: 24 }

  it('treats every overflow value except `visible` as hiding', () => {
    for (const value of ['hidden', 'clip', 'scroll', 'auto', 'overlay']) {
      const byId = mount({ el: { overflowX: value } })
      expect(oracleOverflowHidesText(byId.el, EL_BOX)).toBe(true)
      vi.restoreAllMocks()
    }
  })

  it('treats `visible` — and a style that carries no overflow at all — as hiding nothing', () => {
    expect(oracleOverflowHidesText(mount({ el: { overflowX: 'visible' } }).el, EL_BOX)).toBe(false)
    vi.restoreAllMocks()
    // The empty string means "this style object has no such longhand", not any
    // CSS value. A partial stub must not be able to manufacture a truncation.
    expect(oracleOverflowHidesText(mount({ el: { overflowX: '', overflow: '' } }).el, EL_BOX)).toBe(
      false,
    )
  })

  it('falls back to the `overflow` shorthand when the longhand is absent', () => {
    expect(
      oracleOverflowHidesText(mount({ el: { overflowX: '', overflow: 'hidden' } }).el, EL_BOX),
    ).toBe(true)
  })

  it('does NOT read `text-overflow`, which is inert without a hiding overflow', () => {
    // CSS UI applies text-overflow only to a block container whose overflow is
    // other than `visible`, so an ellipsis declaration alone can never be the
    // thing that hides a glyph. v1.12's own element proves the other direction:
    // it computes `text-overflow: clip` and is not truncated at all.
    const byId = mount({ el: { overflowX: 'visible', textOverflow: 'ellipsis' } })
    expect(oracleOverflowHidesText(byId.el, EL_BOX)).toBe(false)
  })

  it('sees an ancestor whose clip rect actually cuts the element', () => {
    const byId = mount(
      { el: { overflowX: 'visible' }, outer: { overflowX: 'hidden' } },
      {
        left: 0,
        top: 0,
        width: 20,
        height: 40,
      },
    )
    // The element runs to x=34; the scrollport stops at x=20.
    expect(oracleOverflowHidesText(byId.el, EL_BOX)).toBe(true)
  })

  it('ignores a hiding ancestor that contains the element whole — the `html` case', () => {
    // This is why the ancestor term intersects rather than merely asking whether
    // a clip exists. `index.css` puts `overflow: hidden` on html, body AND
    // #root, so every anchor in every live capture has three clipping
    // ancestors; a CSS-only test would be a tautology and v1.12 would do
    // nothing at all.
    const byId = mount({
      el: { overflowX: 'visible' },
      host: { overflowX: 'hidden' },
      outer: { overflowX: 'hidden' },
    })
    expect(oracleOverflowHidesText(byId.el, EL_BOX)).toBe(false)
  })

  it('does not call a sub-pixel ancestor overhang a clip either', () => {
    const byId = mount(
      { el: { overflowX: 'visible' }, outer: { overflowX: 'hidden' } },
      {
        left: 0,
        top: 0,
        width: 33.7,
        height: 40,
      },
    )
    expect(oracleOverflowHidesText(byId.el, EL_BOX)).toBe(false)
    expect(oracleOverflowHidesText(byId.el, EL_BOX, 0.1)).toBe(true)
  })
})

// ── generating a box at all (ANCHORS.md v1.11) ───────────────────────────────

describe('oracleGeneratesBox', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
  })

  function mount(styles: Record<string, FakeStyle>) {
    document.body.innerHTML = `<div id="host"><span id="el">x</span></div>`
    const byId: Record<string, Element> = {
      host: document.getElementById('host') as HTMLElement,
      el: document.getElementById('el') as HTMLElement,
    }
    const resolved = new Map<Element, FakeStyle>()
    for (const key of Object.keys(styles)) resolved.set(byId[key], styles[key])
    vi.spyOn(window, 'getComputedStyle').mockImplementation(((el: Element) => {
      return { ...BASE_STYLE, ...(resolved.get(el) || {}) } as unknown as CSSStyleDeclaration
    }) as typeof window.getComputedStyle)
    return byId
  }

  it('is false for `display: none` on the element itself', () => {
    expect(oracleGeneratesBox(mount({ el: { display: 'none' } }).el)).toBe(false)
  })

  it('is false for `display: none` on an ancestor', () => {
    // getComputedStyle reports the descendant's *own* display here — whatever it
    // was authored as — so the walk is necessary rather than defensive.
    expect(oracleGeneratesBox(mount({ host: { display: 'none' } }).el)).toBe(false)
  })

  // The distinction v1.11 says must not be blurred: these three all generate
  // boxes, stay anchors, and report `visible: false`. Collapsing this predicate
  // into `oracleIsVisible` would delete the carousel's snapped-out panels and
  // v1.7's opacity case from every snapshot that contains them.
  it('is true for `visibility: hidden`, which still generates a box', () => {
    expect(oracleGeneratesBox(mount({ el: { visibility: 'hidden' } }).el)).toBe(true)
  })

  it('is true for `opacity: 0`, on the element and on an ancestor', () => {
    expect(oracleGeneratesBox(mount({ el: { opacity: '0' } }).el)).toBe(true)
    vi.restoreAllMocks()
    expect(oracleGeneratesBox(mount({ host: { opacity: '0' } }).el)).toBe(true)
  })

  it('is true for an ordinary element (the control)', () => {
    expect(oracleGeneratesBox(mount({}).el)).toBe(true)
  })
})

/**
 * The v1.12 case, in the shape P3.10 measured it: a callout emoji whose glyph is
 * **wider than its content box** and painted in full anyway.
 *
 * ```
 * border box 24 · padding 4/4 + border 1/1 → content box 14 · text advance 19
 * ```
 *
 * `19 − 14 = 5` is five pixels of overhang, so v1.3 ruling 1's predicate calls it
 * truncated on its own. Whether it *is* depends entirely on `#outer`, which is
 * what each test below varies.
 */
function mountCallout(outer: FakeStyle, outerRect?: { left: number; width: number }) {
  document.documentElement.className = 'light'
  document.body.innerHTML = `
    <div id="outer">
      <div id="root" data-oracle-id="callout-node">
        <span id="emoji" data-oracle-id="callout-emoji">💡</span>
      </div>
    </div>`

  const byId: Record<string, Element> = {
    outer: document.getElementById('outer') as HTMLElement,
    root: document.getElementById('root') as HTMLElement,
    emoji: document.getElementById('emoji') as HTMLElement,
  }
  stubRect(byId.outer, {
    left: outerRect ? outerRect.left : 0,
    top: 0,
    width: outerRect ? outerRect.width : 400,
    height: 40,
  })
  stubRect(byId.root, { left: 0, top: 0, width: 400, height: 40 })
  // Border box 24, running from x=10 to x=34.
  stubRect(byId.emoji, { left: 10, top: 8, width: 24, height: 24 })

  const styles = new Map<Element, FakeStyle>([
    [byId.outer, outer],
    [
      byId.emoji,
      {
        // `justify-center` spills the glyph 2.5px past each edge of the content
        // box and nothing here hides it — `text-overflow` computes to `clip`.
        overflowX: 'visible',
        overflowY: 'visible',
        textOverflow: 'clip',
        paddingLeft: '4px',
        paddingRight: '4px',
        borderLeftWidth: '1px',
        borderRightWidth: '1px',
      },
    ],
  ])

  vi.spyOn(window, 'getComputedStyle').mockImplementation(((
    el: Element,
    pseudo?: string | null,
  ) => {
    if (pseudo) return { ...BASE_STYLE, content: 'none' } as unknown as CSSStyleDeclaration
    return { ...BASE_STYLE, ...(styles.get(el) || {}) } as unknown as CSSStyleDeclaration
  }) as typeof window.getComputedStyle)

  vi.spyOn(document, 'createRange').mockImplementation(
    () =>
      ({
        selectNodeContents: () => {},
        getClientRects: () => [{ width: 19 }],
        getBoundingClientRect: () => ({ width: 19 }),
      }) as unknown as Range,
  )

  return byId
}

describe('v1.12: `clipped` is truncation, so it consults overflow', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    document.documentElement.className = ''
  })

  function emoji(): OracleSnapshot['anchors'][number] {
    const { anchors } = extractSnapshot({ surface: 'callout-node', root: 'callout-node' })
    return anchors.find((a) => a.id === 'callout-emoji') as OracleSnapshot['anchors'][number]
  }

  it('does not call a glyph painted in full truncated', () => {
    mountCallout({ overflowX: 'visible' })
    const a = emoji()

    // The overhang is real and still reported — `text_width` is the unclipped
    // advance and that has not changed. Only the verdict has.
    expect(a.text_width).toBe(19)
    expect(a.clipped).toBe(false)
  })

  it('still does not, when the only clipping ancestor contains the glyph whole', () => {
    // index.css puts `overflow: hidden` on html, body AND #root, so every anchor
    // in every live capture has clipping ancestors. If the ancestor term asked
    // only whether one exists, this would be `true` and v1.12 would be a no-op
    // on every surface the app can capture.
    mountCallout({ overflowX: 'hidden' })
    expect(emoji().clipped).toBe(false)
  })

  it('does call it truncated when an ancestor’s clip rect actually cuts it', () => {
    // The glyph runs to x=34; the scrollport stops at x=20.
    mountCallout({ overflowX: 'hidden' }, { left: 0, width: 20 })
    expect(emoji().clipped).toBe(true)
  })

  it('does call it truncated when the element’s own box hides the overflow', () => {
    // The control for the whole ruling, and the shape every archived
    // `clipped: true` has: a `truncate` span, `overflow: hidden` on itself.
    mountRow()
    const { anchors } = extractSnapshot({ surface: 'git-status-row' })
    const name = anchors.find((a) => a.id === 'git-row-name')

    expect(name?.text_width).toBe(186.5)
    expect(name?.clipped).toBe(true)
  })
})

/**
 * The v1.11 case: three anchors that are all invisible for three different
 * reasons, only **one** of which stops the element generating a box.
 */
function mountBoxTree(styles: Record<string, FakeStyle>) {
  document.documentElement.className = 'light'
  document.body.innerHTML = `
    <div id="root" data-oracle-id="probe-root">
      <span id="gone" data-oracle-id="probe-gone">tick</span>
      <span id="faded" data-oracle-id="probe-faded">faded</span>
      <span id="veiled" data-oracle-id="probe-veiled">veiled</span>
      <div id="wrap"><span id="buried" data-oracle-id="probe-buried">buried</span></div>
    </div>`

  const byId: Record<string, Element> = {
    root: document.getElementById('root') as HTMLElement,
    gone: document.getElementById('gone') as HTMLElement,
    faded: document.getElementById('faded') as HTMLElement,
    veiled: document.getElementById('veiled') as HTMLElement,
    wrap: document.getElementById('wrap') as HTMLElement,
    buried: document.getElementById('buried') as HTMLElement,
  }
  stubRect(byId.root, { left: 0, top: 0, width: 300, height: 20 })
  // A `display: none` element measures 0×0 in a real engine; the point of v1.11
  // is that it must not reach the snapshot as a zero-rect record at all.
  stubRect(byId.gone, { left: 0, top: 0, width: 0, height: 0 })
  stubRect(byId.faded, { left: 10, top: 2, width: 40, height: 16 })
  stubRect(byId.veiled, { left: 60, top: 2, width: 40, height: 16 })
  stubRect(byId.wrap, { left: 110, top: 2, width: 40, height: 16 })
  stubRect(byId.buried, { left: 110, top: 2, width: 40, height: 16 })

  const resolved = new Map<Element, FakeStyle>()
  for (const key of Object.keys(styles)) resolved.set(byId[key], styles[key])
  vi.spyOn(window, 'getComputedStyle').mockImplementation(((
    el: Element,
    pseudo?: string | null,
  ) => {
    if (pseudo) return { ...BASE_STYLE, content: 'none' } as unknown as CSSStyleDeclaration
    return { ...BASE_STYLE, ...(resolved.get(el) || {}) } as unknown as CSSStyleDeclaration
  }) as typeof window.getComputedStyle)

  return byId
}

const BOX_TREE = {
  gone: { display: 'none' },
  faded: { opacity: '0' },
  veiled: { visibility: 'hidden' },
}

describe('v1.11: an element that generates no box is not an anchor', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    document.documentElement.className = ''
  })

  function snapshot(): OracleSnapshot {
    return extractSnapshot({ surface: 'oracle-probe', root: 'probe-root' })
  }

  it('skips a `display: none` anchor rather than emitting a zero-rect record', () => {
    mountBoxTree(BOX_TREE)
    const ids = snapshot().anchors.map((a) => a.id)

    // GPUI has no such element at all, so emitting it put a delta on *anchor
    // presence* — a structural difference caused by the contract, on a resting
    // cell, that reads exactly like a port bug.
    expect(ids).not.toContain('probe-gone')
    expect(ids).toEqual(['probe-root', 'probe-faded', 'probe-veiled', 'probe-buried'])
  })

  it('skips an anchor buried under a `display: none` ancestor too', () => {
    mountBoxTree({ ...BOX_TREE, wrap: { display: 'none' } })
    expect(snapshot().anchors.map((a) => a.id)).toEqual([
      'probe-root',
      'probe-faded',
      'probe-veiled',
    ])
  })

  // The distinction that must not be blurred. These two are how the carousel's
  // snapped-out panels and v1.7's opacity case are caught, and both would
  // vanish from every snapshot if the skip were keyed on `visible` instead.
  it('keeps an `opacity: 0` anchor, with visible false (the control)', () => {
    mountBoxTree(BOX_TREE)
    const faded = snapshot().anchors.find((a) => a.id === 'probe-faded')

    expect(faded).toBeDefined()
    expect(faded?.visible).toBe(false)
    expect(faded?.bounds).toEqual({ x: 10, y: 2, w: 40, h: 16 })
  })

  it('keeps a `visibility: hidden` anchor, with visible false (the control)', () => {
    mountBoxTree(BOX_TREE)
    const veiled = snapshot().anchors.find((a) => a.id === 'probe-veiled')

    expect(veiled).toBeDefined()
    expect(veiled?.visible).toBe(false)
    expect(veiled?.bounds).toEqual({ x: 60, y: 2, w: 40, h: 16 })
  })

  it('refuses a capture whose root generates no box, by name', () => {
    mountBoxTree({ ...BOX_TREE, root: { display: 'none' } })
    expect(() => snapshot()).toThrow(/root anchor "probe-root" generates no box/)
  })

  it('changes nothing when every anchor has a box (the control)', () => {
    mountBoxTree({})
    expect(snapshot().anchors.map((a) => a.id)).toEqual([
      'probe-root',
      'probe-gone',
      'probe-faded',
      'probe-veiled',
      'probe-buried',
    ])
  })
})

describe('extractSnapshot', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    document.documentElement.className = ''
  })

  it('emits a v1 snapshot rooted on the anchor it was told to root on', () => {
    mountRow('dark')
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

  // ── the mislabelled capture, end to end ───────────────────────────────────

  it('makes it impossible to get a snapshot back for a contradicted theme', () => {
    // This is the live defect, reproduced: the app is in light — a reload
    // undid an earlier theme switch — and the operator asks for the dark cell.
    // The old code answered with a snapshot labelled `dark` carrying light
    // values. Nothing downstream could have caught that, so nothing may come
    // back at all: the binding stays undefined, not merely wrong.
    mountRow('light')

    let snapshot: OracleSnapshot | undefined
    expect(() => {
      snapshot = extractSnapshot({
        surface: 'git-status-row',
        state: { width: 1100, theme: 'dark', content: 'overflow', flags: ['hover'] },
      })
    }).toThrow(/state\.theme/)
    expect(snapshot).toBeUndefined()
  })

  it('refuses a light label on a dark document as well', () => {
    mountRow('dark')
    expect(() => extractSnapshot({ surface: 'git-status-row', state: { theme: 'light' } })).toThrow(
      /document being measured is "dark"/,
    )
  })

  it('returns a snapshot when the label matches the document (the control)', () => {
    // The other half of the claim: the check is a *filter on wrong labels*, not
    // a blanket refusal of declared state. A guard that failed this would have
    // taken the reference capture path out entirely.
    mountRow('dark')
    const snapshot = extractSnapshot({
      surface: 'git-status-row',
      state: { width: 1100, theme: 'dark', content: 'overflow', flags: ['hover'] },
    })

    expect(snapshot.state).toEqual({
      width: 1100,
      theme: 'dark',
      content: 'overflow',
      flags: ['hover'],
    })
    expect(snapshot.anchors).toHaveLength(5)
    expect(snapshot.anchors[0].id).toBe('git-row-item')
  })

  it('takes the theme from the document when none is declared', () => {
    mountRow('dark')
    expect(extractSnapshot({ surface: 'git-status-row' }).state.theme).toBe('dark')
  })

  it('does not change any emitted value — the archived snapshots still compare', () => {
    // The check runs at capture time and must not touch the wire shape: a
    // labelled capture and an unlabelled one over the same document have to be
    // byte-identical, or `native/oracle/runs/`’s archived pairs stop matching.
    mountRow('dark')
    const declared = extractSnapshot({ surface: 'git-status-row', state: { theme: 'dark' } })
    const derived = extractSnapshot({ surface: 'git-status-row' })

    expect(JSON.stringify(declared)).toBe(JSON.stringify(derived))
  })
})

// ── the v1.7 opacity gap, made unable to produce a wrong answer ──────────────
//
// ANCHORS.md v1.7 says `visible` is false at zero opacity on the element or any
// ancestor. This side implements all of that sentence; `crowbar-driver`
// implements it for *anchored* ancestors only, and cannot do better without
// patching the pinned vendor tree. So a cell driven with a transparent
// **unanchored** ancestor produces `visible: false` here against `true` there —
// a delta on every anchor, caused by the contract rather than by the port.
// `corpus/001-opacity-ancestor-visible.md` records that as a standing
// restriction; these tests are the restriction made mechanical.

describe('the v1.7 unanchored-opacity precondition', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    document.documentElement.className = ''
  })

  it('refuses the NavStack case — a transparent layer above the root anchor', () => {
    // The motivating scenario, and the one the driver's own v1.7 work did not
    // reach: `opacity-0` on the layer *holding* the surface. Nothing may come
    // back, exactly as for a contradicted theme — a snapshot that reached an
    // archive would become evidence for a port bug that does not exist.
    mountProbeTree({ outer: { opacity: '0' } })

    let snapshot: OracleSnapshot | undefined
    expect(() => {
      snapshot = extractSnapshot(PROBE)
    }).toThrow(/would not be comparable/)
    expect(snapshot).toBeUndefined()
  })

  it('refuses an unanchored transparent ancestor between two anchors', () => {
    // The driver's probe tree, one for one:
    // `anchor_root("root", div().child(div().opacity(0.).child(anchor("child"))))`
    // reports both anchors visible; this side reports neither.
    mountProbeTree({ gap: { opacity: '0' } })
    expect(() => extractSnapshot(PROBE)).toThrow(/would not be comparable/)
  })

  it('searches all the way to <html>, where oracleIsVisible also stops', () => {
    // The region has to be the one `oracleIsVisible` reads, or the guard covers
    // less than the field it is protecting. `<body>` and `<html>` are ancestors
    // like any other and neither can carry an anchor id.
    mountProbeTree({ body: { opacity: '0' } })
    expect(() => extractSnapshot(PROBE)).toThrow(/would not be comparable/)

    vi.restoreAllMocks()
    mountProbeTree({ html: { opacity: '0' } })
    expect(() => extractSnapshot(PROBE)).toThrow(/would not be comparable/)
  })

  it('names the offending element, the anchor under it, and the corpus entry', () => {
    // The error is the whole signal: it is read by an operator mid-capture, in
    // a bridge where nothing else is being watched.
    const els = mountProbeTree({ gap: { opacity: '0' } })
    els.gap.setAttribute('class', 'nav-layer')

    let message = ''
    try {
      extractSnapshot(PROBE)
    } catch (error) {
      message = String((error as Error).message)
    }

    expect(message).toContain('<div id="gap" class="nav-layer">')
    expect(message).toContain('opacity 0')
    // The anchor the walk reached it from — `#gap` sits between the root and
    // the child, so the child is what names it.
    expect(message).toContain('probe-child')
    expect(message).toContain('corpus/001-opacity-ancestor-visible.md')
    // The way out, which is what the corpus entry tells a caller to do.
    expect(message).toContain('Put the opacity on an anchor')
  })

  it('lets a fully opaque tree through untouched (the control)', () => {
    // Half the claim. A guard that failed this would refuse every honest
    // capture, which is a worse outcome than the gap it exists to cover.
    mountProbeTree({})
    const snapshot = extractSnapshot(PROBE)

    expect(snapshot.anchors.map((a) => a.id)).toEqual(['probe-root', 'probe-child'])
    expect(snapshot.anchors.every((a) => a.visible)).toBe(true)
    // And it adds nothing to the wire shape: `native/oracle/runs/` holds pairs
    // that must still compare, and a field only one side writes is worse than
    // no coverage at all.
    expect(Object.keys(snapshot).sort()).toEqual(['anchors', 'root', 'schema', 'state', 'surface'])
    for (const anchor of snapshot.anchors) {
      expect(Object.keys(anchor).sort()).toEqual([
        'bg',
        'border',
        'bounds',
        'id',
        'radius',
        'visible',
      ])
    }
  })

  it('does NOT trip on an anchored transparent ancestor above the root', () => {
    // The case that is comparable, and therefore must still capture. The driver
    // folds an anchored ancestor's opacity in on `enter`, so both sides agree —
    // and they agree on `false`, which is the answer a reader wants.
    mountProbeTree({ outer: { opacity: '0' } }, { outer: true })
    const snapshot = extractSnapshot(PROBE)

    expect(snapshot.anchors.map((a) => a.id)).toEqual(['probe-root', 'probe-child'])
    expect(snapshot.anchors.every((a) => a.visible)).toBe(false)
  })

  it('does NOT trip on an anchored transparent ancestor between anchors', () => {
    // The corpus entry's advice for exercising the field at all is "put the
    // opacity on an anchor". If the guard blocked that, the field would be
    // untestable and the advice would be wrong.
    mountProbeTree({ gap: { opacity: '0' } }, { gap: true })
    const snapshot = extractSnapshot(PROBE)

    expect(snapshot.anchors.map((a) => a.id)).toEqual(['probe-root', 'probe-gap', 'probe-child'])
    expect(snapshot.anchors.find((a) => a.id === 'probe-root')?.visible).toBe(true)
    expect(snapshot.anchors.find((a) => a.id === 'probe-gap')?.visible).toBe(false)
    expect(snapshot.anchors.find((a) => a.id === 'probe-child')?.visible).toBe(false)
  })

  it('does NOT trip on a merely translucent unanchored ancestor', () => {
    // Exact zero, not `< 1`, and deliberately: both `visible` implementations
    // test for zero per level, so `opacity: 0.5` on an unanchored ancestor
    // moves neither side's answer. There is no disagreement to prevent, and
    // refusing here would throw away legitimate captures on a tree where
    // partial opacity is ordinary — the row fixture's own indent guide sits at
    // 0.9.
    mountProbeTree({ outer: { opacity: '0.5' }, gap: { opacity: '0.01' } })
    const snapshot = extractSnapshot(PROBE)

    expect(snapshot.anchors.every((a) => a.visible)).toBe(true)
  })

  it('changes nothing about the gate surface it is not aimed at', () => {
    // The archived pairs are captures of this row. The guard runs on every one
    // of them and has to be invisible: same anchors, same values, same keys.
    mountRow('dark')
    const snapshot = extractSnapshot({ surface: 'git-status-row', state: { theme: 'dark' } })

    expect(snapshot.anchors).toHaveLength(5)
    expect(snapshot.anchors[0].bounds).toEqual({ x: 0, y: 0, w: 320, h: 24 })
    expect(snapshot.anchors[0].bg).toBe('#ff000080')
  })
})

// ── surface scope: a capture holds the surface's own anchors (P2.11) ─────────
//
// The live defect, in the shape it was hit in. `resizable`'s root anchor is
// `resize-group` — the IDE shell's own group — so the walk reached the whole
// sidebar through it: the carousel, the file rows and ten git rows, 81 anchors
// with repeated ids against the native surface's four. The differ refused it by
// name (`anchor id 'git-row-item' appears twice`) and was right to. The fixture
// below is that tree, small enough to read and with the two properties that
// matter kept intact: a *foreign* anchored subtree under the root, and repeated
// ids inside it.

/**
 * The IDE shell's group with the sidebar carousel — and two file rows and two
 * git rows — living inside its sidebar panel.
 *
 * `omit` drops one anchor the surface declares; `duplicate` renders a second
 * element carrying a declared id. Both are how the two refusals are reached
 * without hand-writing a second fixture.
 */
function mountShell(options?: { omit?: string; duplicate?: string }) {
  document.documentElement.className = 'light'
  const omit = options?.omit
  const duplicate = options?.duplicate
  const anchor = (id: string) => (id === omit ? '' : ` data-oracle-id="${id}"`)
  const twin = (id: string) =>
    duplicate === id ? `<div data-oracle-id="${id}" data-twin="true"></div>` : ''

  document.body.innerHTML = `
    <div id="group"${anchor('resize-group')}>
      <div id="sidebar"${anchor('resize-panel-sidebar')}>
        <div id="scrollport"${anchor('carousel-scrollport')}>
          <div${anchor('carousel-panel-workspaces')}></div>
          <div${anchor('carousel-panel-chats')}></div>
          <div${anchor('carousel-panel-files')}>
            <div data-oracle-id="file-row-item"><span data-oracle-id="file-row-name"></span></div>
            <div data-oracle-id="file-row-item"><span data-oracle-id="file-row-name"></span></div>
          </div>
          <div${anchor('carousel-panel-git')}>
            <div data-oracle-id="git-row-item"><span data-oracle-id="git-row-name"></span></div>
            <div data-oracle-id="git-row-item"><span data-oracle-id="git-row-name"></span></div>
          </div>
          ${twin('carousel-panel-git')}
        </div>
      </div>
      <div id="handle"${anchor('resize-handle')}></div>
      <div id="content"${anchor('resize-panel-content')}></div>
      ${twin('resize-panel-content')}
    </div>`

  const at = (id: string) => document.getElementById(id) as HTMLElement
  stubRect(at('group'), { left: 0, top: 0, width: 1000, height: 600 })
  stubRect(at('sidebar'), { left: 0, top: 0, width: 300, height: 600 })
  stubRect(at('scrollport'), { left: 0, top: 0, width: 300, height: 600 })
  stubRect(at('handle'), { left: 300, top: 0, width: 1, height: 600 })
  stubRect(at('content'), { left: 301, top: 0, width: 699, height: 600 })

  vi.spyOn(window, 'getComputedStyle').mockImplementation(
    ((_el: Element, pseudo?: string | null) =>
      ({
        ...BASE_STYLE,
        ...(pseudo ? { content: 'none' } : {}),
      }) as unknown as CSSStyleDeclaration) as typeof window.getComputedStyle,
  )
}

const RESIZABLE_ANCHORS = [
  'resize-group',
  'resize-panel-sidebar',
  'resize-handle',
  'resize-panel-content',
]

describe('a surface captures its own anchors, not everything under its root', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    document.documentElement.className = ''
  })

  it('keeps only the declared anchors when the root contains a foreign subtree', () => {
    mountShell()
    const snapshot = extractSnapshot({ surface: 'resizable' })

    expect(snapshot.anchors.map((a) => a.id)).toEqual(RESIZABLE_ANCHORS)
    // Geometry is untouched by the narrowing: the same elements, measured by
    // the same code, relative to the same root.
    expect(snapshot.anchors[0].bounds).toEqual({ x: 0, y: 0, w: 1000, h: 600 })
    expect(snapshot.anchors[2].bounds).toEqual({ x: 300, y: 0, w: 1, h: 600 })

    // And the control, without which the assertion above proves only that some
    // reduction happened. The identical tree, captured as a surface that
    // declares nothing, still walks the whole subtree — including the repeated
    // ids that made the live capture unusable.
    const unscoped = extractSnapshot({ surface: 'ide-shell', root: 'resize-group' })
    const ids = unscoped.anchors.map((a) => a.id)

    expect(ids.length).toBeGreaterThan(RESIZABLE_ANCHORS.length)
    expect(ids.filter((id) => id === 'git-row-item')).toHaveLength(2)
    expect(ids).toContain('carousel-scrollport')
  })

  it('does not care that the foreign subtree repeats ids — it is not the surface', () => {
    // The `resizable` capture above succeeded with two `git-row-item`s and two
    // `file-row-item`s in the document. Ambiguity is only ambiguity among the
    // anchors being compared; refusing on a foreign one would make the surface
    // uncapturable for a reason that has nothing to do with it.
    mountShell()
    expect(document.querySelectorAll('[data-oracle-id="git-row-item"]')).toHaveLength(2)
    expect(extractSnapshot({ surface: 'resizable' }).anchors).toHaveLength(4)
  })

  it('scopes one tree two ways, by surface — the carousel inside the same shell', () => {
    // The same document, captured as the surface *inside* it. This is the list
    // `native/oracle/runs/p2.3-carousel` holds, which was reached by hand
    // filtering the walk's output to the `carousel-*` ids; it now falls out of
    // the declaration, so a re-capture reproduces those archived pairs rather
    // than depending on whoever runs it repeating the filter.
    mountShell()
    const snapshot = extractSnapshot({ surface: 'sidebar-carousel' })

    expect(snapshot.root).toBe('carousel-scrollport')
    expect(snapshot.anchors.map((a) => a.id)).toEqual([
      'carousel-scrollport',
      'carousel-panel-workspaces',
      'carousel-panel-chats',
      'carousel-panel-files',
      'carousel-panel-git',
    ])
  })

  it('refuses a capture missing an anchor the surface declares', () => {
    // The failure mode a filter introduces and the reason this one refuses: a
    // capture that is quietly smaller than the surface still compares, and
    // proves less every time it shrinks.
    mountShell({ omit: 'resize-handle' })

    let snapshot: OracleSnapshot | undefined
    expect(() => {
      snapshot = extractSnapshot({ surface: 'resizable' })
    }).toThrow(/declares the anchor\(s\) resize-handle, and the document has none/)
    // Not "three anchors instead of four" — nothing at all.
    expect(snapshot).toBeUndefined()
  })

  it('refuses two elements carrying one declared id, rather than picking the first', () => {
    mountShell({ duplicate: 'resize-panel-content' })

    let snapshot: OracleSnapshot | undefined
    expect(() => {
      snapshot = extractSnapshot({ surface: 'resizable' })
    }).toThrow(/more than one element carrying the declared anchor id\(s\) resize-panel-content ×2/)
    expect(snapshot).toBeUndefined()

    // The differ's own words for why, on the surface that hit it: this is the
    // refusal moved to the point of capture, where the tree is still in front
    // of whoever caused it.
    mountShell({ duplicate: 'carousel-panel-git' })
    expect(() => extractSnapshot({ surface: 'sidebar-carousel' })).toThrow(
      /no way to say which of them it compared/,
    )
  })

  it('refuses a declaration that cannot be satisfied — an id declared twice', () => {
    // Reached through the helper rather than the table, which has no such
    // entry: without this the second copy would report as *missing* however the
    // document is built, and the message would send a reader to the DOM.
    expect(() =>
      oracleSelectDeclaredAnchors([], 'resizable', ['resize-group', 'resize-group']),
    ).toThrow(/declares the anchor "resize-group" twice/)
  })

  it('pins the root as part of the set, so two captures cannot disagree on it', () => {
    mountShell()
    expect(() => extractSnapshot({ surface: 'resizable', root: 'resize-panel-sidebar' })).toThrow(
      /is rooted on "resize-group", not on "resize-panel-sidebar"/,
    )
  })

  it('leaves a surface that declares nothing exactly as it was', () => {
    // The 63 archived `git-status-row` / `file-tree-row` pairs were captured on
    // this path and have to keep comparing byte-for-byte. `git-status-row`
    // declares no set — its anchors are a function of the cell, not of the
    // surface (`git-row-guide-{n}` per depth level, `git-row-dir` only when the
    // fixture has one) — so the walk hands the loop the identical array.
    expect(oracleSurfaceScope('git-status-row')).toBeNull()
    expect(oracleSurfaceScope('file-tree-row')).toBeNull()

    mountRow('dark')
    const snapshot = extractSnapshot({ surface: 'git-status-row', state: { theme: 'dark' } })

    expect(snapshot.anchors.map((a) => a.id)).toEqual([
      'git-row-item',
      'git-row-guide-0',
      'git-row-button',
      'git-row-name',
      'git-row-badge',
    ])
    expect(snapshot.anchors[0].bounds).toEqual({ x: 0, y: 0, w: 320, h: 24 })
    expect(snapshot.anchors[0].bg).toBe('#ff000080')
  })
})

// ── injectability ────────────────────────────────────────────────────────────

describe('extractSnapshotSource', () => {
  afterEach(() => {
    vi.restoreAllMocks()
    document.body.innerHTML = ''
    document.documentElement.className = ''
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

  it('carries the theme check into the injected script, where the capture happens', () => {
    // The only path that matters for the live defect. The bridge evaluates this
    // IIFE in the page — if `oracleNormalizeState` were left out of
    // `ORACLE_RUNTIME`, or the guard lived in the module rather than in a
    // serialised helper, the module path above would be green and every real
    // capture would still hand back a mislabelled snapshot.
    mountRow('light')
    const source = extractSnapshotSource({
      surface: 'git-status-row',
      state: { theme: 'dark' },
    })

    expect(() => (0, eval)(source)).toThrow(/document being measured is "light"/)
  })

  it('carries the opacity precondition into the injected script as well', () => {
    // Same reasoning as the theme check above, and the same failure if it is
    // missed: the bridge evaluates this IIFE in the page, so a guard left out
    // of `ORACLE_RUNTIME` is a `ReferenceError` there and green everywhere
    // else. Matching the guard's own wording is deliberate — a ReferenceError
    // would throw too, and would pass a bare `.toThrow()`.
    mountProbeTree({ outer: { opacity: '0' } })
    const source = extractSnapshotSource(PROBE)

    expect(() => (0, eval)(source)).toThrow(/would not be comparable/)
  })

  it('still captures through the injected script when every ancestor is opaque', () => {
    mountProbeTree({})
    const emitted = JSON.parse((0, eval)(extractSnapshotSource(PROBE)) as string)

    expect(emitted).toEqual(JSON.parse(JSON.stringify(extractSnapshot(PROBE))))
  })

  it('carries the surface scope into the injected script, where the capture happens', () => {
    // The `resizable` capture only ever happens through this path — a live IDE
    // shell over an `execute_js` bridge. A scope helper left out of
    // `ORACLE_RUNTIME` would be a `ReferenceError` in the page and green in
    // every module-path test above, so this asserts the *narrowed* list rather
    // than merely that something came back.
    mountShell()
    const options = { surface: 'resizable' }
    const emitted = JSON.parse((0, eval)(extractSnapshotSource(options)) as string)

    expect(emitted.anchors.map((a: { id: string }) => a.id)).toEqual(RESIZABLE_ANCHORS)
    expect(emitted).toEqual(JSON.parse(JSON.stringify(extractSnapshot(options))))
  })

  it('still produces a snapshot through the injected script when the label agrees', () => {
    mountRow('dark')
    const options = { surface: 'git-status-row', state: { theme: 'dark' as const } }
    const emitted = JSON.parse((0, eval)(extractSnapshotSource(options)) as string)

    expect(emitted.state.theme).toBe('dark')
    expect(emitted).toEqual(JSON.parse(JSON.stringify(extractSnapshot(options))))
  })
})
