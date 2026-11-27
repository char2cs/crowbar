import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  applyStreamedValue,
  freshDecorations,
  settleFreshGeneration,
  settleFreshWord,
} from '@/features/agent/transcript/plate/streaming-value-patch'
import { stableBlockCount } from '@/features/agent/transcript/plate/streaming-value-blocks'
import { resumedFadeDelay } from '@/features/agent/transcript/plate/streaming-fade-delay'

describe('applyStreamedValue performance', () => {
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

    expect(editor.children[0]).toBe(firstBlock)
    expect((editor.children[0] as { children: { text: string }[] }).children[0]!.text).toBe(
      'CORRUPTED',
    )
    expect(editor.children.length).toBe(2)
  })
})

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
      if (range.chatFresh === undefined) continue
      fresh.add(text.slice(range.anchor.offset, range.focus.offset))
    }
  }
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

describe('applyStreamedValue: a completing mark does not re-fade the rest of the paragraph', () => {
  it('marks only the genuinely new text fresh when a bold span closes mid-paragraph', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('The cat is very '),
    })

    applyStreamedValue(editor, chatMarkdownToValue('The cat is very **special**'))

    expect(editor.api.string([0])).toBe('The cat is very special')
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
    expect(fadeWords(editor)).toEqual([])
  })
})

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
    expect(before.length).toBeGreaterThan(1)
    const generation = before[0]?.chatFresh
    const totalWords = before[0]?.chatFreshWordTotal
    expect(typeof generation).toBe('number')
    expect(typeof totalWords).toBe('number')

    settleFreshWord(editor, generation as number, 0, totalWords as number)

    const after = freshDecorations(editor, entry) as Range[]
    const others = after.filter((r) => r.chatFreshWordIndex !== 0)
    expect(others.length).toBeGreaterThan(0)
    for (const r of others) {
      expect(r.chatFresh).toBe(generation)
    }
  })
})

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
      expect(delay).toBeCloseTo(Math.max(atBirth[i]! - 208, -260), 5)
    })
  })

  it('mounts a run older than its whole window already finished, never blank', () => {
    const now = vi.spyOn(performance, 'now')
    now.mockReturnValue(1_000)
    const animated = animatedRangesFor('one two three four')

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

describe('applyStreamedValue: the patched document matches a fresh parse at every prefix', () => {
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

describe('applyStreamedValue: a rebuild does not re-fade text already on screen', () => {
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

    applyStreamedValue(editor, chatMarkdownToValue(`${seen} It gained a tail.\n\nSecond`))

    const fresh = fadeWords(editor).join('')
    expect(fresh).not.toContain(seen)
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

    expect(longOps).toBe(shortOps)
    expect(longOps).toBeLessThanOrEqual(3)
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

describe('applyStreamedValue: an ordered list streams as cheaply as prose', () => {
  const orderedList = (items: number) =>
    Array.from({ length: items }, (_, i) => `${i + 1}. item ${i} body text`).join('\n')

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
    const eight = worstFlushOps(orderedList(8))
    const twentyFour = worstFlushOps(orderedList(24))
    expect(twentyFour).toBeLessThanOrEqual(eight + 2)
    expect(twentyFour).toBeLessThanOrEqual(8)
  })

  it('leaves the document agreeing with a fresh parse, list numbering aside', () => {
    const markdown = orderedList(10)
    const tokens = markdown.match(/\s*\S+/g) ?? []
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(tokens.slice(0, 2).join('')),
    })
    for (let i = 4; i <= tokens.length; i += 2) {
      applyStreamedValue(editor, chatMarkdownToValue(tokens.slice(0, i).join('')))
    }

    const parsed = chatMarkdownToValue(markdown)
    expect(stableBlockCount(editor.children as never, parsed)).toBe(parsed.length)
  })

  it('costs an ordered list no more per delta than a bulleted one', () => {
    const bulleted = Array.from({ length: 14 }, (_, i) => `- item ${i} body text`).join('\n')
    expect(worstFlushOps(orderedList(14))).toBeLessThanOrEqual(worstFlushOps(bulleted) + 2)
  })
})

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
