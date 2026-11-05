import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  applyStreamedValue,
  freshDecorations,
  resumedFadeDelay,
  settleFreshGeneration,
  settleFreshWord,
  splitIntoWords,
  staggerDelay,
} from '@/features/agent/transcript/plate/streaming-value-patch'
import { stableBlockCount } from '@/features/agent/transcript/plate/streaming-value-blocks'

describe('splitIntoWords', () => {
  it('concatenates back to the exact original string', () => {
    const cases = [
      'one two three',
      ' leading space',
      'trailing space ',
      'double  space',
      'newline\nseparated\nwords',
      'no-spaces-at-all',
      '',
      '   ',
    ]
    for (const text of cases) {
      expect(splitIntoWords(text).join('')).toBe(text)
    }
  })

  it('splits plain prose into one chunk per word', () => {
    expect(splitIntoWords('the CLI uses argparse')).toEqual(['the ', 'CLI ', 'uses ', 'argparse'])
  })

  it('treats a whitespace-only string as a single chunk', () => {
    expect(splitIntoWords('   ')).toEqual(['   '])
  })

  it('treats an empty string as a single (empty) chunk', () => {
    expect(splitIntoWords('')).toEqual([''])
  })
})

describe('staggerDelay', () => {
  it('is zero for the first word and increases with word index', () => {
    expect(staggerDelay(0, 5)).toBe(0)
    expect(staggerDelay(1, 5)).toBeGreaterThan(staggerDelay(0, 5))
    expect(staggerDelay(2, 5)).toBeGreaterThan(staggerDelay(1, 5))
  })

  it('steps by 30ms for a chunk short enough that never exceeds the cap', () => {
    // 11 words * 30ms = 330ms, just over the 320ms cap — so 10 words is the
    // largest count that still gets the full, uncompressed 30ms step.
    expect(staggerDelay(1, 10)).toBe(30)
    expect(staggerDelay(9, 10)).toBe(270)
  })

  // The per-word STEP scales down rather than the delay being capped, so a long chunk never
  // fades in as one abrupt batch after the first few words.
  it('never gives two different words in the same chunk an identical delay, however long the chunk', () => {
    for (const total of [15, 40, 100]) {
      const delays = Array.from({ length: total }, (_, i) => staggerDelay(i, total))
      expect(new Set(delays).size).toBe(total)
    }
  })

  it('keeps the whole cascade within MAX_STAGGER_MS regardless of chunk length', () => {
    for (const total of [2, 15, 40, 100, 1000]) {
      expect(staggerDelay(total - 1, total)).toBeLessThanOrEqual(320)
    }
  })

  it('is zero for a single-word chunk', () => {
    expect(staggerDelay(0, 1)).toBe(0)
  })
})

describe('applyStreamedValue performance', () => {
  // Re-canonicalizing a stable block on every token is O(n^2) over a long reply and starves the
  // main thread; corrupting a stable block proves later calls never re-examine it.
  it('never re-examines a block once a later block has started streaming', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('First paragraph.'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('First paragraph.\n\nSecond'))
    expect(editor.children.length).toBe(2)

    const firstBlock = editor.children[0] as { children: { text: string }[] }
    firstBlock.children[0]!.text = 'CORRUPTED'

    applyStreamedValue(editor, chatMarkdownToValue('First paragraph.\n\nSecond thought'))

    // Read fresh from the editor, not the captured reference — a
    // stale-reference check would pass even if the block got torn down and
    // rebuilt, since the old JS object it points to keeps its mutation.
    expect(editor.children[0]).toBe(firstBlock)
    expect((editor.children[0] as { children: { text: string }[] }).children[0]!.text).toBe(
      'CORRUPTED',
    )
    expect(editor.children.length).toBe(2)
  })
})

// closeAssistantTurn can reconcile a streamed tail against the hook's own text. A non-append
// edit must patch only the diverging tail: replacing the block re-fades unchanged text and
// briefly shows the old and new prefix together.
function leaves(editor: ReturnType<typeof createPlateEditor>): { text: string; fresh: boolean }[] {
  const block = editor.children[0] as { children: { text: string }[] }
  const fresh = new Set<string>()
  for (const entry of editor.api.nodes({ at: [] })) {
    const [node] = entry
    if (typeof (node as { text?: string }).text !== 'string') continue
    const text = (node as { text: string }).text
    for (const range of freshDecorations(editor, entry) as {
      anchor: { offset: number }
      focus: { offset: number }
      chatFresh?: number
    }[]) {
      // Animated ranges only — a settled run keeps emitting an inert one to
      // hold its boundary for the words still fading after it.
      if (range.chatFresh === undefined) continue
      fresh.add(text.slice(range.anchor.offset, range.focus.offset))
    }
  }
  // The fade is a decoration now, so "which text is fresh" is read from the
  // ranges rather than from split-up document leaves; the block itself stays
  // whatever the markdown parse produced.
  return block.children.flatMap((child) => {
    const text = child.text
    const covered = [...fresh].filter((f) => f.length > 0 && text.includes(f))
    if (covered.length === 0) return [{ text, fresh: false }]
    const freshText = covered.join('')
    const at = text.lastIndexOf(freshText)
    if (at < 0) return [{ text, fresh: true }]
    const out: { text: string; fresh: boolean }[] = []
    if (at > 0) out.push({ text: text.slice(0, at), fresh: false })
    out.push({ text: freshText, fresh: true })
    if (at + freshText.length < text.length)
      out.push({ text: text.slice(at + freshText.length), fresh: false })
    return out
  })
}
describe('applyStreamedValue: reconciliation replaces part of a paragraph', () => {
  it('keeps the untouched prefix out of the fresh-marked (re-animated) leaves', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('The cat sat on the mat'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('The cat sat on the rug'))

    const result = leaves(editor)
    const fullText = result.map((l) => l.text).join('')
    expect(fullText).toBe('The cat sat on the rug')

    const freshText = result
      .filter((l) => l.fresh)
      .map((l) => l.text)
      .join('')
    const settledText = result
      .filter((l) => !l.fresh)
      .map((l) => l.text)
      .join('')
    expect(freshText).toBe('rug')
    expect(settledText).toBe('The cat sat on the ')
  })

  it('never leaves the stale (pre-reconciliation) wording anywhere in the document', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('The cat sat on the mat'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('The cat sat on the rug'))

    const fullText = leaves(editor)
      .map((l) => l.text)
      .join('')
    expect(fullText).not.toContain('mat')
    // Not a substring accident either — the whole document text is exactly
    // the reconciled sentence, nothing appended alongside it.
    expect(fullText).toBe('The cat sat on the rug')
  })

  it('still falls back to a full, freshly-animated replace when nothing is shared at all', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('hello'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('goodbye'))

    const result = leaves(editor)
    expect(result.map((l) => l.text).join('')).toBe('goodbye')
    expect(result.every((l) => l.fresh)).toBe(true)
  })
})

// An open markdown span renders as literal text until it closes and gains its mark, a prop
// change no text edit expresses. The paragraph may be rebuilt, but only the word whose markup
// resolved may fade again, never the already-settled rest.
describe('applyStreamedValue: a completing mark does not re-fade the rest of the paragraph', () => {
  it('marks only the genuinely new text fresh when a bold span closes mid-paragraph', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('The cat is very '),
    })

    applyStreamedValue(editor, chatMarkdownToValue('The cat is very **special**'))

    expect(editor.api.string([0])).toBe('The cat is very special')
    // The parser trims the initial fragment's trailing space, so the space
    // separating "very" and "special" is genuinely new in the flattened diff
    // too — only "The cat is very" (no trailing space) was already visible.
    expect(fadeWords(editor).join('')).toBe(' special')
  })

  it('marks only the genuinely new text fresh when an inline code span closes mid-paragraph', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Run the '),
    })

    applyStreamedValue(editor, chatMarkdownToValue('Run the `build` command'))

    expect(editor.api.string([0])).toBe('Run the build command')
    expect(fadeWords(editor).join('')).not.toContain('Run the ')
  })

  // A mark resolving BEFORE the block's end shrinks the flattened text there, so a prefix-only
  // comparison would misread all the settled text after it as fresh.
  it('does not re-fade already-visible text after a NON-trailing mark completes mid-paragraph', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Use `code and more text that was already visible'),
    })

    applyStreamedValue(
      editor,
      chatMarkdownToValue('Use `code` and more text that was already visible'),
    )

    expect(editor.api.string([0])).toBe('Use code and more text that was already visible')
    // Nothing in this reply grew — only an earlier code span's closing
    // backtick landed — so there is no genuinely new text to fade at all.
    expect(fadeWords(editor)).toEqual([])
  })
})

// Every word split from one chunk shares a `generation`, so settling must be per word: the
// first word's `animationend` (zero stagger) must not retire words still waiting on their delay.
describe('settleFreshWord: a per-word cascade does not retire on its first word alone', () => {
  it('keeps later words animating after only the fastest (zero-delay) word settles', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(''),
    })

    applyStreamedValue(editor, chatMarkdownToValue('one two three four'))

    const entry = [...editor.api.nodes({ at: [] })].find(
      ([node]) => typeof (node as { text?: string }).text === 'string',
    )
    if (!entry) throw new Error('no text leaf found')

    type Range = {
      chatFresh?: number
      chatFreshWordIndex?: number
      chatFreshWordTotal?: number
    }
    const before = freshDecorations(editor, entry) as Range[]
    // More than one word split out of the chunk — otherwise this test proves
    // nothing about a cascade.
    expect(before.length).toBeGreaterThan(1)
    const generation = before[0]?.chatFresh
    const totalWords = before[0]?.chatFreshWordTotal
    expect(typeof generation).toBe('number')
    expect(typeof totalWords).toBe('number')

    // Settle exactly the word at index 0 — the one a real browser's
    // `animationend` would always fire first, since its own stagger delay is
    // always zero.
    settleFreshWord(editor, generation as number, 0, totalWords as number)

    const after = freshDecorations(editor, entry) as Range[]
    const others = after.filter((r) => r.chatFreshWordIndex !== 0)
    expect(others.length).toBeGreaterThan(0)
    for (const r of others) {
      // Still carries chatFresh (still animating), NOT rendered as the
      // instantly-inert CHAT_FRESH_HELD shape — settling one word must not
      // retire the rest of the chunk's cascade.
      expect(r.chatFresh).toBe(generation)
    }
  })
})

/** Every fade range currently emitted, as the text each one covers. */
function fadeWords(editor: ReturnType<typeof createPlateEditor>): string[] {
  const out: string[] = []
  for (const entry of editor.api.nodes({ at: [] })) {
    const [node] = entry
    if (typeof (node as { text?: string }).text !== 'string') continue
    const text = (node as { text: string }).text
    for (const range of freshDecorations(editor, entry) as {
      anchor: { offset: number }
      focus: { offset: number }
      chatFresh?: number
    }[]) {
      if (range.chatFresh === undefined) continue
      out.push(text.slice(range.anchor.offset, range.focus.offset))
    }
  }
  return out
}

/** Slate operations one call actually costs. Every operation runs the whole
 *  plugin stack's `apply`/`normalizeNode` overrides — measured at ~1.4ms each
 *  in Chrome with this plugin set — so this count IS the frame budget. */
function countOps(editor: ReturnType<typeof createPlateEditor>, run: () => void): number {
  const target = editor as unknown as { apply: (op: unknown) => void }
  const original = target.apply.bind(editor)
  let ops = 0
  target.apply = (op: unknown) => {
    ops++
    original(op)
  }
  try {
    run()
  } finally {
    target.apply = original
  }
  return ops
}

// PERFORMANCE, live-measured (Chrome 152): streaming rendered at ~24fps
// against Claude Code Desktop's ~60fps, because the fade was written into the
// DOCUMENT as one leaf per word — so an ordinary chunk cost one Slate
// operation PER WORD, and each word's `animationend` cost another. At ~1.4ms
// per operation (ListPlugin's `apply`/`normalizeNode` overrides dominate) an
// 8-word chunk spent ~11ms of a 16.7ms frame before React rendered anything.
//
// These assert the COST, not just the output — a slow implementation produces
// identical text. Both fail on the pre-decoration design.
// slate-react keys leaves by position in a split rebuilt every delta, so fading spans remount
// constantly; under `animation-fill-mode: both` each remount must resume the fade, not restart
// it from invisible. The elapsed part is applied at MOUNT (`resumedFadeDelay`); see below.
describe('resumedFadeDelay: a remount resumes the fade rather than restarting it', () => {
  afterEach(() => vi.restoreAllMocks())

  const animatedRangesFor = (markdown: string) => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(''),
    })
    applyStreamedValue(editor, chatMarkdownToValue(markdown))
    const entry = [...editor.api.nodes({ at: [] })].find(
      ([node]) => typeof (node as { text?: string }).text === 'string',
    )
    if (!entry) throw new Error('no text leaf found')
    const ranges = freshDecorations(editor, entry) as unknown as Record<string, unknown>[]
    return ranges.filter((r) => r.chatFresh !== undefined)
  }

  it('hands a span mounting mid-fade the remainder of its own wait', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const animated = animatedRangesFor('one two three four')
    expect(animated.length).toBeGreaterThan(1)

    const atBirth = animated.map((r) => resumedFadeDelay(r as never) as number)
    now.mockReturnValue(1_208)
    const remounted = animated.map((r) => resumedFadeDelay(r as never) as number)

    remounted.forEach((delay, i) => {
      // 208ms further along than it was — never back at the original wait,
      // which is what a restart from `opacity: 0` would be.
      expect(delay).toBeCloseTo(Math.max(atBirth[i]! - 208, -260), 5)
    })
  })

  it('mounts a run older than its whole window already finished, never blank', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const animated = animatedRangesFor('one two three four')

    // Past lead (150) + the cascade's own cap (320) + the fade (260), with no
    // `animationend` ever delivered — exactly what a remount produces. A delay
    // at minus the fade's own length means the animation is already over, so
    // the word paints opaque on its very first frame instead of blank.
    now.mockReturnValue(1_000 + 736)
    for (const range of animated) {
      expect(resumedFadeDelay(range as never)).toBe(-260)
    }
  })

  it('never asks for a delay that outlives the fade, however old the run is', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const animated = animatedRangesFor('one two three four')
    now.mockReturnValue(1_000 + 60_000)
    for (const range of animated) {
      expect(resumedFadeDelay(range as never)).toBeGreaterThanOrEqual(-260)
    }
  })
})

// A decoration carrying the clock can never satisfy `isTextDecorationsEqual`, so every fading
// block would re-render per delta and rewriting a running `animation-delay` repaints settled
// text. The elapsed part therefore never lives in the decoration.
describe('freshDecorations: what a decoration carries does not move with the clock', () => {
  afterEach(() => vi.restoreAllMocks())

  it('emits identical ranges however much time has passed', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(''),
    })
    applyStreamedValue(editor, chatMarkdownToValue('one two three four'))
    const entry = [...editor.api.nodes({ at: [] })].find(
      ([node]) => typeof (node as { text?: string }).text === 'string',
    )
    if (!entry) throw new Error('no text leaf found')

    const first = freshDecorations(editor, entry)
    for (const t of [1_001, 1_017, 1_400, 9_999, 60_000]) {
      now.mockReturnValue(t)
      expect(freshDecorations(editor, entry)).toEqual(first)
    }
  })
})

// A matching TRAILING block is only "unchanged so far": `knownStablePrefix` must not latch it,
// or later edits are dropped (tables are one block over many deltas). The invariant: a patched
// editor holds exactly what a fresh parse holds, at every prefix.
describe('applyStreamedValue: the patched document matches a fresh parse at every prefix', () => {
  /** Block types, and a table's row/cell counts — enough to catch a table that
   *  never materialised, a row that never landed, or a stale paragraph. */
  const shapeOf = (value: unknown[]) =>
    value
      .map((node) => {
        const n = node as { type?: string; children?: unknown[] }
        if (n.type !== 'table') return n.type ?? '?'
        const rows = (n.children ?? []) as { children?: unknown[] }[]
        return `table(${rows.map((r) => (r.children ?? []).length).join(',')})`
      })
      .join('|')

  const streamCharByChar = (markdown: string) => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(''),
    })
    const divergences: string[] = []
    for (let i = 1; i <= markdown.length; i++) {
      const text = markdown.slice(0, i)
      const fresh = chatMarkdownToValue(text)
      applyStreamedValue(editor, fresh)
      const got = shapeOf(editor.children as unknown[])
      const want = shapeOf(fresh as unknown[])
      if (got !== want) {
        divergences.push(`at ${i} (${JSON.stringify(text.slice(-20))}): ${got} != ${want}`)
      }
    }
    return { editor, divergences }
  }

  it('never falls behind while a markdown table streams in', () => {
    const markdown = [
      'Intro line.',
      '',
      '| Name | Model | Scale |',
      '| --- | --- | --- |',
      '| Postgres | relational | vertical |',
      '| Cassandra | wide column | horizontal |',
      '| Redis | key value | memory |',
      '',
      'Closing line.',
      '',
    ].join('\n')

    const { editor, divergences } = streamCharByChar(markdown)
    expect(divergences).toEqual([])
    // Belt and braces: the table genuinely materialised rather than the whole
    // stream having stayed paragraphs that merely agreed with each other.
    expect(shapeOf(editor.children as unknown[])).toBe('p|table(3,3,3,3)|p')
  })

  it('never falls behind while a bold-titled list streams in', () => {
    const markdown = [
      'Here they are:',
      '',
      '1. **Slow start** — the window doubles each round trip.',
      '2. **Congestion avoidance** — it then grows linearly.',
      '',
      'That is all.',
      '',
    ].join('\n')

    const { divergences } = streamCharByChar(markdown)
    expect(divergences).toEqual([])
  })
})

// One per-frame flush can extend the open paragraph AND start the next, the one shape that hits
// the tail rebuild. Reinserted blocks must keep their settled state, or a finished paragraph
// fades in again as the next one begins.
describe('applyStreamedValue: a rebuild does not re-fade text already on screen', () => {
  /** Every generation currently fading, retired as a real `animationend`
   *  would — so what `fadeWords` reports afterwards is only what the NEXT
   *  delta marked fresh, never a leftover from setting the scene. */
  const settleEverything = (editor: ReturnType<typeof createPlateEditor>) => {
    for (const entry of editor.api.nodes({ at: [] })) {
      const [node] = entry
      if (typeof (node as { text?: string }).text !== 'string') continue
      for (const range of freshDecorations(editor, entry) as { chatFresh?: number }[]) {
        if (typeof range.chatFresh === 'number') settleFreshGeneration(editor, range.chatFresh)
      }
    }
  }

  it('leaves the finished paragraph alone when the next one starts in the same delta', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(''),
    })
    const seen = 'The first paragraph is complete.'
    applyStreamedValue(editor, chatMarkdownToValue(seen))
    settleEverything(editor)
    expect(fadeWords(editor).join('')).toBe('')

    // ONE delta that extends the open paragraph and opens the next — what a
    // frame's worth of coalesced deltas routinely looks like.
    applyStreamedValue(editor, chatMarkdownToValue(`${seen} It gained a tail.\n\nSecond`))

    const fresh = fadeWords(editor).join('')
    // Text the reader has already been shown is not marked fresh again...
    expect(fresh).not.toContain(seen)
    // ...while everything genuinely new still fades in.
    expect(fresh).toContain('It gained a tail.')
    expect(fresh).toContain('Second')
  })

  it('re-fades nothing at all when the delta only opens a new block', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(''),
    })
    applyStreamedValue(editor, chatMarkdownToValue('Only paragraph.'))
    settleEverything(editor)
    applyStreamedValue(editor, chatMarkdownToValue('Only paragraph.\n\nNext'))

    const fresh = fadeWords(editor).join('')
    expect(fresh).not.toContain('Only paragraph.')
    expect(fresh).toContain('Next')
  })
})

describe('applyStreamedValue: cost does not scale with the words in a chunk', () => {
  it('spends the same handful of operations on a 2-word and a 60-word append', () => {
    const short = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('start'),
    })
    const long = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('start'),
    })
    const manyWords = Array.from({ length: 60 }, (_, i) => `word${i}`).join(' ')

    const shortOps = countOps(short, () =>
      applyStreamedValue(short, chatMarkdownToValue('start two words')),
    )
    const longOps = countOps(long, () =>
      applyStreamedValue(long, chatMarkdownToValue(`start ${manyWords}`)),
    )

    // The old design emitted one insert_node per word: 2 vs 60. Appending is
    // a single `insert_text` now, whatever the word count.
    expect(longOps).toBe(shortOps)
    expect(longOps).toBeLessThanOrEqual(3)
    // ...and the text still all arrived, still all faded.
    expect(long.api.string([0])).toBe(`start ${manyWords}`)
    expect(fadeWords(long).join('')).toBe(` ${manyWords}`)
  })

  it('appends at a flat cost no matter how long the reply has already grown', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('First paragraph.'),
    })
    let text = 'First paragraph.'
    const costs: number[] = []
    for (let i = 0; i < 40; i++) {
      text += ` chunk${i} of more text`
      costs.push(countOps(editor, () => applyStreamedValue(editor, chatMarkdownToValue(text))))
    }

    // Flat, not growing: the tail of a long reply costs exactly what its head
    // did. This is what a per-word document split could never give, since a
    // later chunk also had to be compared against an ever-longer run of
    // still-unsettled word leaves.
    expect(Math.max(...costs)).toBe(Math.min(...costs))
    expect(editor.api.string([0])).toBe(text)
  })
})

describe('applyStreamedValue: a large jump still animates as one unit', () => {
  it('fades a many-word insertion as a single range, not one per word', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [{ type: 'p', id: 'irrelevant', children: [{ text: '' }] }],
    })
    const words = Array.from({ length: 200 }, (_, i) => `word${i}`).join(' ')

    applyStreamedValue(editor, chatMarkdownToValue(words))

    expect(editor.api.string([0])).toBe(words)
    // Past WORD_SPLIT_CAP the stagger is imperceptible, so it collapses to
    // one range rather than 200 DOM spans — still animated, just as one unit.
    expect(fadeWords(editor)).toEqual([words])
  })

  it('still splits a small insertion per word, staggered as before', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [{ type: 'p', id: 'irrelevant', children: [{ text: '' }] }],
    })

    applyStreamedValue(editor, chatMarkdownToValue('five short words here'))

    expect(fadeWords(editor)).toEqual(['five ', 'short ', 'words ', 'here'])
  })
})

// PERFORMANCE, live-reported after the decoration fix landed ("fps still
// drops"), then isolated in Chrome to ORDERED lists specifically — bulleted
// ones were always fine.
//
// `@platejs/list`'s `normalizeListStart` renumbers ordered items from their
// position and DELETES `listStart` from the first item, whose `1` is
// implicit; the markdown parse always emits it. That single derived prop on
// that single block made the document and a fresh parse of the very same text
// disagree forever, so `stableBlockCount` returned 0 and every flush tore the
// whole list down and reinserted it — and each reinsertion re-entered the same
// renumbering pass, which is why the cost grew with the square of the list.
// Measured before the fix: 24 items cost 121 Slate operations per flush, a
// 42.5ms median frame and one 571ms freeze.
describe('applyStreamedValue: an ordered list streams as cheaply as prose', () => {
  const orderedList = (items: number) =>
    Array.from({ length: items }, (_, i) => `${i + 1}. item ${i} body text`).join('\n')

  /** Streams `markdown` in small deltas, as Codex's transport does, and
   *  reports the worst single flush's Slate operation count. */
  function worstFlushOps(markdown: string, wordsPerDelta = 2): number {
    const tokens = markdown.match(/\s*\S+/g) ?? []
    const steps: string[] = []
    let acc = ''
    tokens.forEach((token, i) => {
      acc += token
      if ((i + 1) % wordsPerDelta === 0) steps.push(acc)
    })
    if (acc !== steps.at(-1)) steps.push(acc)

    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(steps[0]!),
    })
    let worst = 0
    for (let i = 1; i < steps.length; i++) {
      worst = Math.max(
        worst,
        countOps(editor, () => applyStreamedValue(editor, chatMarkdownToValue(steps[i]!))),
      )
    }
    return worst
  }

  it('does not re-insert the whole list on every delta', () => {
    // The tell, directly: a longer list must not cost proportionally more per
    // flush. Before the fix these were 45, 120 and 325.
    const eight = worstFlushOps(orderedList(8))
    const twentyFour = worstFlushOps(orderedList(24))
    expect(twentyFour).toBeLessThanOrEqual(eight + 2)
    expect(twentyFour).toBeLessThanOrEqual(8)
  })

  it('leaves the document agreeing with a fresh parse, list numbering aside', () => {
    const markdown = orderedList(10)
    // Streamed in deltas, not applied in one go: the divergence only builds
    // up once the list plugin has normalized a list it inserted itself.
    const tokens = markdown.match(/\s*\S+/g) ?? []
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(tokens.slice(0, 2).join('')),
    })
    for (let i = 4; i <= tokens.length; i += 2) {
      applyStreamedValue(editor, chatMarkdownToValue(tokens.slice(0, i).join('')))
    }

    // Every block matches, which is what keeps the next delta on the cheap
    // path. `listStart` is excluded by IGNORED_KEYS precisely because the
    // list plugin owns it and the parse cannot agree with it.
    const parsed = chatMarkdownToValue(markdown)
    expect(stableBlockCount(editor.children as never, parsed)).toBe(parsed.length)
  })

  it('costs an ordered list no more per delta than a bulleted one', () => {
    const bulleted = Array.from({ length: 14 }, (_, i) => `- item ${i} body text`).join('\n')
    expect(worstFlushOps(orderedList(14))).toBeLessThanOrEqual(worstFlushOps(bulleted) + 2)
  })
})

// A resolving mark (bold, inline code) must patch in place: a block replace reassigns the
// NodeIdPlugin `id`, remounting the block and resetting its fade, so a block resolving several
// marks in a row would never finish fading in.
describe('applyStreamedValue: a resolving mark patches in place, not a block replace', () => {
  it('keeps the block id stable when a bold span closes mid-paragraph', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('**Fourth item titl'),
    })
    const idBefore = (editor.children[0] as { id?: string }).id

    applyStreamedValue(editor, chatMarkdownToValue('**Fourth item title**'))

    expect((editor.children[0] as { id?: string }).id).toBe(idBefore)
    expect(editor.children[0]).toMatchObject({
      children: [{ bold: true, text: 'Fourth item title' }],
    })
  })

  it('keeps the block id stable when a WHOLE-line code span closes', () => {
    // Unlike a code span in the MIDDLE of a paragraph (which splits one leaf
    // into three — a genuine shape change, see the fallback test below), a
    // code span that is the paragraph's entire content stays a single leaf
    // both before (unclosed backtick, parsed as literal text) and after
    // (closed, parsed as one `code` leaf) — patchable in place.
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('`getConfi'),
    })
    const idBefore = (editor.children[0] as { id?: string }).id

    applyStreamedValue(editor, chatMarkdownToValue('`getConfig()`'))

    expect((editor.children[0] as { id?: string }).id).toBe(idBefore)
    expect(editor.children[0]).toMatchObject({
      children: [{ code: true, text: 'getConfig()' }],
    })
  })

  it('falls back to a full replace when a code span splits a leaf in the middle of a paragraph', () => {
    // A structural change (the paragraph gains two new leaf boundaries where
    // it had none) cannot be expressed leaf-by-leaf — the id changing here is
    // correct, not a regression: there is no cheaper way to carve a new leaf
    // out of the middle of an existing one.
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Requires an `includ'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('Requires an `include`.'))

    expect(editor.children[0]).toMatchObject({
      children: [{ text: 'Requires an ' }, { code: true, text: 'include' }, { text: '.' }],
    })
  })

  it('keeps a preceding, untouched list item stable across a LATER item resolving its own mark', () => {
    // The scenario a real reply hits: item 3 already settled; item 4's own
    // bold title is what's resolving now. Item 3 must not remount either.
    const markdown = (fourthTitle: string) => `3. **Classification rule**\n\n4. ${fourthTitle}`

    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(markdown('**Fourth item titl')),
    })
    const item3IdBefore = (editor.children[0] as { id?: string }).id

    applyStreamedValue(editor, chatMarkdownToValue(markdown('**Fourth item title**')))

    expect((editor.children[0] as { id?: string }).id).toBe(item3IdBefore)
    expect(editor.children[1]).toMatchObject({
      children: [{ bold: true, text: 'Fourth item title' }],
    })
  })

  it('still falls back to a full replace when the block SHAPE actually changes', () => {
    // A structural change (here: the paragraph gains a second leaf where it
    // had one) cannot be expressed leaf-by-leaf — the block replace fallback
    // is still correct and still exercised.
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('plain text'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('plain `code` text'))

    expect(editor.children[0]).toMatchObject({
      children: [{ text: 'plain ' }, { code: true, text: 'code' }, { text: ' text' }],
    })
  })
})
