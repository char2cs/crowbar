import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  applyStreamedValue,
  freshDecorations,
  settleFreshWord,
  splitIntoWords,
  stableBlockCount,
  staggerDelay,
  trailingTextDivergence,
} from '@/features/agent/transcript/plate/streaming-value-patch'

const p = (text: string, extra: Record<string, unknown> = {}) => ({
  type: 'p',
  id: 'irrelevant',
  children: [{ text, ...extra }],
})

describe('stableBlockCount', () => {
  it('is the full length when every block matches', () => {
    const a = [p('one'), p('two')]
    const b = [p('one'), p('two')]
    expect(stableBlockCount(a, b)).toBe(2)
  })

  it('stops at the first block that differs', () => {
    const a = [p('one'), p('two')]
    const b = [p('one'), p('two, growing')]
    expect(stableBlockCount(a, b)).toBe(1)
  })

  it('ignores id — a fresh reparse mints a new one for every block', () => {
    const a = [{ type: 'p', id: 'a1', children: [{ text: 'same' }] }]
    const b = [{ type: 'p', id: 'a2', children: [{ text: 'same' }] }]
    expect(stableBlockCount(a, b)).toBe(1)
  })

  it('is 0 when nothing in common, and never exceeds the shorter array', () => {
    expect(stableBlockCount([p('a')], [p('b'), p('c')])).toBe(0)
    expect(stableBlockCount([p('a'), p('b')], [p('a')])).toBe(1)
  })
})

describe('trailingTextDivergence', () => {
  it('is a pure append (keep = the whole of prev) for a plain growing paragraph', () => {
    const prevText = p('Building a').children![0]!
    expect(trailingTextDivergence(prevText, p('Building a CLI').children![0]!)).toEqual({
      keep: (prevText.text as string).length,
      replacement: ' CLI',
    })
  })

  it('is a pure append walking down through unchanged wrapper elements', () => {
    const prev = { type: 'p', children: [{ text: 'partial' }] }
    const next = { type: 'p', children: [{ text: 'partial text' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 7, replacement: ' text' })
  })

  // Regression: turn/message.go's closeAssistantTurn can reconcile a
  // streamed message's tail against the terminating hook's own text when
  // they disagree — the shape a pure append cannot describe at all. This
  // used to fall all the way to null (a full block replace, re-fading
  // content that hadn't actually changed); now the shared prefix is kept
  // and only the genuinely different tail is reported as replaced.
  it('keeps the shared prefix and reports only the diverging tail when reconciliation changes wording partway through', () => {
    const prev = { type: 'p', children: [{ text: 'The cat sat on the mat' }] }
    const next = { type: 'p', children: [{ text: 'The cat sat on the rug' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({
      keep: 'The cat sat on the '.length,
      replacement: 'rug',
    })
  })

  it('reports keep: 0 when nothing at all is shared, rather than refusing structurally', () => {
    const prev = { type: 'p', children: [{ text: 'hello' }] }
    const next = { type: 'p', children: [{ text: 'goodbye' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 0, replacement: 'goodbye' })
  })

  it('reports an empty replacement when reconciliation only shortens the tail', () => {
    const prev = { type: 'p', children: [{ text: 'hello world' }] }
    const next = { type: 'p', children: [{ text: 'hello' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 5, replacement: '' })
  })

  it('returns null when a mark changed rather than the text growing or reconciling', () => {
    const prev = { type: 'p', children: [{ text: 'run make', code: true }] }
    const next = { type: 'p', children: [{ text: 'run make' }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('returns null when a non-trailing leaf changed', () => {
    const prev = { type: 'p', children: [{ text: 'a' }, { text: 'b' }] }
    const next = { type: 'p', children: [{ text: 'A' }, { text: 'b' }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('recurses into the last child when only the final inline run changed', () => {
    const prev = {
      type: 'p',
      children: [{ text: 'see ' }, { text: 'bold', bold: true }],
    }
    const next = {
      type: 'p',
      children: [{ text: 'see ' }, { text: 'bolder', bold: true }],
    }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 4, replacement: 'er' })
  })

  it('returns null when the child count changed (a new inline run started)', () => {
    const prev = { type: 'p', children: [{ text: 'see ' }] }
    const next = { type: 'p', children: [{ text: 'see ' }, { text: 'bold', bold: true }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('returns null when the block type changed', () => {
    const prev = { type: 'p', children: [{ text: 'x' }] }
    const next = { type: 'h1', children: [{ text: 'x' }] }
    expect(trailingTextDivergence(prev, next)).toBeNull()
  })

  it('keeps everything and replaces nothing for two identical blocks', () => {
    expect(trailingTextDivergence(p('same'), p('same'))).toEqual({ keep: 4, replacement: '' })
  })

  it('ignores id when comparing element props', () => {
    const prev = { type: 'p', id: 'a1', children: [{ text: 'grow' }] }
    const next = { type: 'p', id: 'a2', children: [{ text: 'growing' }] }
    expect(trailingTextDivergence(prev, next)).toEqual({ keep: 4, replacement: 'ing' })
  })
})

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

  // Regression: the first version of this CAPPED the delay per word, so
  // every word past ~10 shared the exact same capped value — a long
  // sentence visibly split into "a handful of words stagger, then the rest
  // fade in as one abrupt batch". Scaling the STEP down instead means every
  // word gets a distinct delay, however long the chunk.
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
  // Regression: once a block is confirmed stable, applyStreamedValue used to
  // re-canonicalize it on every subsequent token anyway (cost proportional to
  // the whole message, not just the growing tail) — an O(n^2) blowup across a
  // long streamed response that starved the main thread and made the fade
  // and scroll-follow both appear to batch into rare, large jumps instead of
  // many small ones. Prove the fix actually SKIPS re-examining a stable block
  // by corrupting it directly and confirming a later call never notices.
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

// Regression: the live bug this fixes. turn/message.go's closeAssistantTurn
// can reconcile a streamed message's tail against the terminating hook's own
// text when they disagree (provider-agnostic — Claude and Codex both go
// through it). Before this fix, applyStreamedValue only recognized a PURE
// append as cheap; anything else fell back to removing the whole block and
// re-inserting it fully fresh-marked — re-fading content that had not
// actually changed, and briefly holding both the old and new copy of the
// UNCHANGED prefix on screen in the same paragraph. Reported live as "text
// repeated on itself, and the smoothing animation is clearly not working".
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

// Regression: the live bug reported as "we're losing markdown styling that
// then reconciles". A markdown span (bold, italic, code, a link) renders as
// plain literal text while it is still open — the closing syntax hasn't
// arrived yet — and only gains its mark once it closes. That mark landing on
// the leaf is a prop change trailingTextDivergence cannot express as a text
// edit, so it fell back to tearing down and reinserting the WHOLE paragraph —
// and the old fallback marked that entire reinserted block as one fresh run,
// re-fading every word in the paragraph that had already faded in and
// settled, not just the one word whose markup just resolved. Visually: a
// long-since-visible sentence flashes and redraws itself the instant any
// **bold** or `code` span anywhere in it completes.
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

  // Regression: `commonPrefixLength` alone can only find a TRAILING
  // divergence correctly. A mark resolving BEFORE the end of the block
  // shrinks the flattened text at that earlier point, so a prefix-only
  // comparison diverges there and misreads everything after it — including
  // long stretches of text that were already fully visible and settled — as
  // fresh, and it flashes/replays its fade-in animation for no reason.
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

// Regression: `generation` used to be the ONLY thing `settleFreshGeneration`
// keyed on, and every word split from one streamed chunk shared it — so the
// FIRST word's own `animationend` (always the one with a zero stagger delay;
// see `staggerDelay`) retired the whole chunk, and every other word — even
// ones whose own delay hadn't elapsed yet — was rendered instantly inert on
// the very next decoration pass instead of playing its own staggered fade.
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
// Regression, root-caused live against a streaming Codex reply: one 16-item
// numbered list produced 1918 `.chat-fresh-text` leaf mounts for 887 animation
// starts, 61 of which never reached `animationend`.
//
// `.chat-fresh-text` animates from `opacity: 0` under `animation-fill-mode:
// both`, and slate-react keys each rendered leaf by its positional index into
// the decoration split this module rebuilds on every delta — so a span is
// unmounted and remounted constantly while its block still streams, and each
// remount used to start the fade AGAIN from invisible. Text was therefore
// visible only if it won a race against the next delta, and retiring a run
// depended entirely on hearing an `animationend` that a remount had already
// cancelled: a run losing that race stayed live and restarted from zero for
// the rest of the turn. That is the reported list whose bullets are on screen
// with nothing underneath them until the turn ends.
describe('freshDecorations: a fade is played against the clock, not the span', () => {
  afterEach(() => vi.restoreAllMocks())

  type FadeRange = { chatFresh?: number; chatFreshDelay?: number; chatFreshHeld?: boolean }

  const streamedLeaf = (text: string) => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(''),
    })
    applyStreamedValue(editor, chatMarkdownToValue(text))
    const entry = [...editor.api.nodes({ at: [] })].find(
      ([node]) => typeof (node as { text?: string }).text === 'string',
    )
    if (!entry) throw new Error('no text leaf found')
    return { editor, entry }
  }

  it('resumes a remounted fade where it was instead of restarting it from invisible', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const { editor, entry } = streamedLeaf('one two three four')

    const atBirth = (freshDecorations(editor, entry) as FadeRange[]).map((r) => r.chatFreshDelay)
    expect(atBirth.length).toBeGreaterThan(1)

    // The same leaf, re-decorated 208ms later (13 whole frames) — what a
    // remount mid-fade gets.
    now.mockReturnValue(1_208)
    const remounted = (freshDecorations(editor, entry) as FadeRange[]).map((r) => r.chatFreshDelay)

    expect(remounted).toHaveLength(atBirth.length)
    remounted.forEach((delay, i) => {
      const born = atBirth[i]
      expect(typeof born).toBe('number')
      expect(typeof delay).toBe('number')
      // Every word is 208ms further along than it was, never back at its
      // original wait — which is what a restart from `opacity: 0` would be.
      expect(delay as number).toBeCloseTo((born as number) - 208, 5)
    })
  })

  it('renders a run inert once its whole window has passed, with no animationend', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const { editor, entry } = streamedLeaf('one two three four')

    expect(
      (freshDecorations(editor, entry) as FadeRange[]).some((r) => r.chatFresh !== undefined),
    ).toBe(true)

    // Past lead (150) + the cascade's own cap (320) + the fade (260). Nothing
    // has settled — no `animationend` was ever delivered, exactly what a
    // remount produces — so the clock is the only thing left to retire it.
    now.mockReturnValue(1_000 + 736)
    const late = freshDecorations(editor, entry) as FadeRange[]
    expect(late.length).toBeGreaterThan(0)
    for (const range of late) {
      expect(range.chatFresh).toBeUndefined()
      expect(range.chatFreshHeld).toBe(true)
    }
  })

  it('never asks for a delay that outlives the fade, however old the run is', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const { editor, entry } = streamedLeaf('one two three four')
    now.mockReturnValue(1_000 + 512)
    for (const range of freshDecorations(editor, entry) as FadeRange[]) {
      if (range.chatFreshDelay === undefined) continue
      expect(range.chatFreshDelay).toBeGreaterThanOrEqual(-260)
    }
  })

  it('is stable within one frame, so an unchanged block is not re-rendered', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const { editor, entry } = streamedLeaf('one two three four')
    const first = freshDecorations(editor, entry)
    now.mockReturnValue(1_009)
    expect(freshDecorations(editor, entry)).toEqual(first)
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

// Regression: a mark (bold, inline code) resolving mid-block used to fall
// straight to the full block-replace fallback — removeNodes + insertNodes on
// the WHOLE block, which reassigns its NodeIdPlugin `id`. A block whose
// rendered identity is keyed by that id remounts on every such edit, which
// resets its `.chat-fresh-text` fade (`animation-fill-mode: both`,
// transcript.css) to its own zero-opacity start — and a block resolving
// several marks in quick succession (a bold list-item title, then an inline
// code span moments later) never gets an uninterrupted 260ms to finish
// fading in. Confirmed live by instrumenting `applyStreamedValue` across a
// realistic delta stream shaped like a real report (a numbered list with
// bold item titles and inline code): the block's `id` changed exactly once
// per item, at the exact token where its own `**`/`` ` `` closed.
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
