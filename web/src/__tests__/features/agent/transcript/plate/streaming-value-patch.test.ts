import { describe, expect, it } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  applyStreamedValue,
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
  const block = editor.children[0] as { children: { text: string; chatFresh?: number }[] }
  return block.children.map((c) => ({ text: c.text, fresh: c.chatFresh !== undefined }))
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

// PERFORMANCE, live-reported: a large turn made the whole app unresponsive
// while streaming. If the main thread ever falls a frame behind, the rAF
// batcher hands back a bigger jump next time (more new blocks landing in one
// insertion) — and splitting every word of a big jump into its own leaf for
// the stagger animation made THAT jump itself more expensive, compounding.
describe('applyStreamedValue: a large jump does not explode into a leaf per word', () => {
  it('lands a many-word insertion as one fresh leaf, not one per word', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [{ type: 'p', id: 'irrelevant', children: [{ text: '' }] }],
    })
    const words = Array.from({ length: 200 }, (_, i) => `word${i}`).join(' ')

    applyStreamedValue(editor, chatMarkdownToValue(words))

    const result = leaves(editor)
    // Full text intact — nothing lost by skipping the per-word split.
    expect(result.map((l) => l.text).join('')).toBe(words)
    // Still animated — just not split into 200 separate leaves for it.
    expect(result.every((l) => l.fresh)).toBe(true)
    expect(result.length).toBeLessThan(5)
  })

  it('still splits a small insertion per word, staggered as before', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: [{ type: 'p', id: 'irrelevant', children: [{ text: '' }] }],
    })

    applyStreamedValue(editor, chatMarkdownToValue('five short words here'))

    const result = leaves(editor)
    expect(result.map((l) => l.text).join('')).toBe('five short words here')
    expect(result.length).toBe(4)
  })
})
