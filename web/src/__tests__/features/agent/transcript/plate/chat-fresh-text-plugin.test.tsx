import { act, render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import { Plate, PlateContent } from 'platejs/react'
import { createPlateEditor } from 'platejs/react'
import { chatComposerPlugins } from '@/features/agent/composer/plate/chat-composer-plugins'
import { chatMarkdownToValue } from '@/features/agent/composer/plate/chat-composer-serialization'
import {
  CHAT_FRESH_DELAY_MARK,
  CHAT_FRESH_MARK,
  applyStreamedValue,
  freshDecorations,
  settleFreshGeneration,
  splitIntoWords,
} from '@/features/agent/transcript/plate/streaming-value-patch'

// `chatComposerPlugins` registers `NodeIdPlugin` (needed for `@platejs/dnd`'s
// hover/drop-target resolution — see attachment-drag-handle.tsx). It only
// assigns `.id` through a real transform, so a REAL editor's `.children`
// carries one after `applyStreamedValue` while a bare `chatMarkdownToValue`
// parse never does. Irrelevant to what these tests check, so it's stripped.
function withoutIds(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(withoutIds)
  if (value && typeof value === 'object') {
    const { id: _id, ...rest } = value as Record<string, unknown>
    return Object.fromEntries(Object.entries(rest).map(([k, v]) => [k, withoutIds(v)]))
  }
  return value
}

type Decoration = Record<string, unknown> & {
  anchor: { path: number[]; offset: number }
  focus: { path: number[]; offset: number }
}

/** Every fade range the editor would render right now, with the text each
 *  one actually covers — the observable the animation is driven by. */
function fades(editor: ReturnType<typeof createPlateEditor>) {
  const out: { text: string; delay: number; generation: number }[] = []
  for (const entry of editor.api.nodes({ at: [] })) {
    const [node] = entry
    if (typeof (node as { text?: string }).text !== 'string') continue
    const text = (node as { text: string }).text
    for (const range of freshDecorations(editor, entry) as Decoration[]) {
      // Only the ANIMATED ranges. A settled run still emits an inert range to
      // hold its boundary for the words still fading after it (see
      // `pruneRuns`); that is deliberately not a fade.
      if (range[CHAT_FRESH_MARK] === undefined) continue
      out.push({
        text: text.slice(range.anchor.offset, range.focus.offset),
        delay: range[CHAT_FRESH_DELAY_MARK] as number,
        generation: range[CHAT_FRESH_MARK] as number,
      })
    }
  }
  return out
}

describe('the fade is a decoration, never a document edit', () => {
  // THE invariant the whole optimization rests on: what streaming leaves in
  // the document is exactly what a plain parse of the same markdown produces.
  // Before this change the fade wrote a LEAF PER WORD into the document, so a
  // streamed paragraph and a parsed one had wildly different shapes until
  // every word's `animationend` had fired and merged them back.
  it('leaves the document byte-identical to a fresh parse of the same markdown', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Building'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('Building a CLI'))

    expect(withoutIds(editor.children)).toEqual(chatMarkdownToValue('Building a CLI'))
    // Specifically: still ONE leaf, not one per appended word.
    expect((editor.children[0]!.children as unknown[]).length).toBe(1)
  })

  it('fades exactly the appended words, each on its own staggered delay', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Building'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('Building a CLI'))

    const shown = fades(editor)
    expect(shown.map((f) => f.text).join('')).toBe(' a CLI')
    // One range per word, and no two words share a delay — the cascade.
    expect(shown.length).toBe(2)
    expect(new Set(shown.map((f) => f.delay)).size).toBe(2)
    expect(shown[1]!.delay).toBeGreaterThan(shown[0]!.delay)
  })

  it('gives each separately-arrived chunk its own generation, so runs do not coalesce', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('one'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('one two'))
    applyStreamedValue(editor, chatMarkdownToValue('one two three'))

    const generations = new Set(fades(editor).map((f) => f.generation))
    expect(generations.size).toBe(2)
  })
})

describe('settleFreshGeneration', () => {
  it('stops the fade being emitted without touching the document', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Building'),
    })
    applyStreamedValue(editor, chatMarkdownToValue('Building a CLI'))
    const before = JSON.parse(JSON.stringify(editor.children))
    expect(fades(editor).length).toBeGreaterThan(0)

    for (const generation of new Set(fades(editor).map((f) => f.generation))) {
      settleFreshGeneration(editor, generation)
    }

    expect(fades(editor)).toEqual([])
    // Settling is bookkeeping only — the old design spent one Slate
    // operation per settling word, which is precisely what made a long
    // reply's tail more expensive than its head.
    expect(editor.children).toEqual(before)
  })

  it('is a safe no-op for a generation that was never recorded', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Building a CLI'),
    })

    expect(() => settleFreshGeneration(editor, 999)).not.toThrow()
    expect(withoutIds(editor.children)).toEqual(chatMarkdownToValue('Building a CLI'))
  })
})

/**
 * REGRESSION, reported live and root-caused in slate's own source: a settled
 * word must not disturb the words still fading beside it.
 *
 * `slate-react` keys every rendered leaf `${textKey}-${i}` where `i` is the
 * POSITIONAL INDEX into the split `Text.decorations` rebuilds from scratch on
 * each render. So dropping one settled range does not just remove its own
 * leaf — it shifts the index of every leaf after it, React sees new keys,
 * unmounts those spans and mounts fresh ones, and a fresh DOM node starts its
 * CSS animation over from zero.
 *
 * While a reply streams, words are settling continuously, so every settle
 * restarted every later still-fading word. Words never got an uninterrupted
 * stretch of real time to finish, and sat at the animation's invisible start
 * state until the stream stopped: literal blank gaps mid-paragraph that
 * healed once the turn ended (many small deltas), or text appearing to pop in
 * with no fade at all (whole-sentence chunks). One mechanism, two symptoms,
 * purely a function of how big the arriving chunks are — so these tests fix
 * the chunk size rather than the provider, and nothing here branches on which
 * provider produced the stream.
 */
describe('a settling word leaves its still-fading neighbours alone', () => {
  const fadeSpans = () => [...document.querySelectorAll<HTMLElement>('.chat-fresh-text')]
  // Word chunks keep their own whitespace (see `splitIntoWords`), so a span
  // for 'two' renders as ' two'.
  const spanFor = (word: string) => fadeSpans().find((s) => s.textContent?.trim() === word)

  async function streamed(initial: string) {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(initial),
    })
    const view = render(
      <Plate editor={editor}>
        <PlateContent readOnly />
      </Plate>,
    )
    // Slate re-renders on the microtask after an external transform, never
    // synchronously with it — see this module's own notes.
    const push = async (markdown: string) => {
      await act(async () => {
        applyStreamedValue(editor, chatMarkdownToValue(markdown))
      })
    }
    return { editor, view, push }
  }

  it('does not remount a fading word when an earlier word settles', async () => {
    const { editor, push } = await streamed('Building')
    await push('Building one') // generation 1
    await push('Building one two') // generation 2

    const before = spanFor('two')
    expect(before).toBeDefined()

    // The first chunk finishes fading for real (`animationend`), then the next
    // chunk arrives — the ordinary streaming sequence, and exactly when the
    // reindex used to happen.
    await act(async () => {
      settleFreshGeneration(editor, 1)
    })
    await push('Building one two three') // generation 3

    // THE ASSERTION: the very same DOM node. A different node means React
    // remounted it, and a remounted span restarts its animation from zero —
    // which is the bug, however identical the two spans look.
    expect(spanFor('two')).toBe(before)
  })

  it('keeps a fading word’s animation-delay stable across an earlier settle', async () => {
    const { editor, push } = await streamed('Building')
    await push('Building one')
    await push('Building alpha beta gamma')

    const delaysBefore = fadeSpans().map((s) => `${s.textContent}@${s.style.animationDelay}`)

    await act(async () => {
      settleFreshGeneration(editor, 1)
    })
    await push('Building alpha beta gamma delta')

    // Every word that was mid-fade keeps the exact delay it was rendered
    // with; a restarted cascade would re-stagger them from the new chunk's
    // clock instead.
    const stillFading = fadeSpans().map((s) => `${s.textContent}@${s.style.animationDelay}`)
    for (const entry of delaysBefore) {
      if (!entry.trim().startsWith('one')) expect(stillFading).toContain(entry)
    }
  })

  it('stops animating a settled word without removing the boundary it holds', async () => {
    const { editor, push } = await streamed('Building')
    await push('Building one')
    await push('Building one two')

    await act(async () => {
      settleFreshGeneration(editor, 1)
    })
    await push('Building one two three')

    // Settled text is no longer animated...
    expect(
      fadeSpans()
        .map((s) => s.textContent)
        .join(''),
    ).not.toContain('one')
    // ...but the document still reads correctly end to end, and the words
    // that ARE still fading are the recent ones.
    expect(editor.api.string([0])).toBe('Building one two three')
  })

  it('releases the boundaries once every word on the line has settled', async () => {
    const { editor, push } = await streamed('Building')
    await push('Building one')
    await push('Building one two')

    await act(async () => {
      settleFreshGeneration(editor, 1)
      settleFreshGeneration(editor, 2)
    })
    await push('Building one two three')

    // Nothing is held open any more: with no fade left to protect, the split
    // collapses — this is what keeps a long reply from carrying every word it
    // ever streamed as a live decoration.
    const held = freshDecorations(editor, [
      editor.children[0]!.children[0]!,
      [0, 0],
    ] as unknown as Parameters<typeof freshDecorations>[1])
    expect(held.length).toBeLessThanOrEqual(splitIntoWords(' three').length)
  })
})

/**
 * REGRESSION, caught live on a 30-item numbered list and reproduced in Chrome:
 * `.chat-fresh-text` spans climbed monotonically to 314 over a single reply
 * and never fell until the stream stopped.
 *
 * Settling is bookkeeping — it deliberately performs no edit — and a block
 * only recomputes its decorations when something re-renders it. While a list
 * streams, only the LAST item is ever re-rendered, so every finished item kept
 * its animated spans in the DOM for the rest of the turn: each one an element
 * still carrying a running `animation` declaration that `fill-mode: both`
 * keeps alive. The old pre-decoration design never had this because settling
 * unset the marks, which merged the leaves back into plain text and removed
 * the spans outright.
 *
 * Bounded, not zero, is the contract: words that genuinely are still fading
 * keep their spans.
 */
describe('finished fades do not pile up in the DOM', () => {
  const listItem = (i: number) => `${i + 1}. item ${i} of the list here`

  it('releases settled spans instead of holding every word of the reply', async () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(listItem(0)),
    })
    render(
      <Plate editor={editor}>
        <PlateContent readOnly />
      </Plate>,
    )

    // Twelve list items stream in, each finishing its fade (as a real browser
    // reports via `animationend`) before the next one starts.
    let markdown = listItem(0)
    for (let i = 1; i < 12; i++) {
      markdown += `\n${listItem(i)}`
      await act(async () => {
        applyStreamedValue(editor, chatMarkdownToValue(markdown))
      })
      await act(async () => {
        for (const generation of liveGenerations(editor)) settleFreshGeneration(editor, generation)
      })
    }
    // One more frame for the coalesced cleanup pass to land.
    await act(async () => {
      await new Promise((resolve) => requestAnimationFrame(() => resolve(null)))
    })

    // Before the fix this was every word of all twelve items — it only ever
    // grew. Bounded well under that is the whole assertion.
    const spans = document.querySelectorAll('.chat-fresh-text').length
    expect(spans).toBeLessThan(10)
    // ...and the reply is all still there, unharmed by the cleanup.
    expect(editor.children.length).toBe(12)
  })
})

/** Generations currently emitting an animated range, from the editor itself. */
function liveGenerations(editor: ReturnType<typeof createPlateEditor>): number[] {
  const out = new Set<number>()
  for (const entry of editor.api.nodes({ at: [] })) {
    if (typeof (entry[0] as { text?: string }).text !== 'string') continue
    for (const range of freshDecorations(editor, entry) as unknown as Record<string, unknown>[]) {
      const generation = range[CHAT_FRESH_MARK]
      if (typeof generation === 'number') out.add(generation)
    }
  }
  return [...out]
}
