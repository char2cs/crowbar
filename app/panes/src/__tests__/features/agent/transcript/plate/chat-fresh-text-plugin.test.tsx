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
} from '@/features/agent/transcript/plate/streaming-value-patch'
import { splitIntoWords } from '@/features/agent/transcript/plate/fresh-word-stagger'

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

function fades(editor: ReturnType<typeof createPlateEditor>) {
  const out: { text: string; delay: number; generation: number }[] = []
  for (const entry of editor.api.nodes({ at: [] })) {
    const [node] = entry
    if (typeof (node as { text?: string }).text !== 'string') continue
    const text = (node as { text: string }).text
    for (const range of freshDecorations(editor, entry) as Decoration[]) {
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
  it('leaves the document byte-identical to a fresh parse of the same markdown', () => {
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue('Building'),
    })

    applyStreamedValue(editor, chatMarkdownToValue('Building a CLI'))

    expect(withoutIds(editor.children)).toEqual(chatMarkdownToValue('Building a CLI'))
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
    const before = structuredClone(editor.children)
    expect(fades(editor).length).toBeGreaterThan(0)

    for (const generation of new Set(fades(editor).map((f) => f.generation))) {
      settleFreshGeneration(editor, generation)
    }

    expect(fades(editor)).toEqual([])
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

describe('a settling word leaves its still-fading neighbours alone', () => {
  const fadeSpans = () => [...document.querySelectorAll<HTMLElement>('.chat-fresh-text')]
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
    const push = async (markdown: string) => {
      await act(async () => {
        applyStreamedValue(editor, chatMarkdownToValue(markdown))
      })
    }
    return { editor, view, push }
  }

  it('does not remount a fading word when an earlier word settles', async () => {
    const { editor, push } = await streamed('Building')
    await push('Building one')
    await push('Building one two')

    const before = spanFor('two')
    expect(before).toBeDefined()

    await act(async () => {
      settleFreshGeneration(editor, 1)
    })
    await push('Building one two three')

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

    expect(
      fadeSpans()
        .map((s) => s.textContent)
        .join(''),
    ).not.toContain('one')
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

    const held = freshDecorations(editor, [
      editor.children[0]!.children[0]!,
      [0, 0],
    ] as unknown as Parameters<typeof freshDecorations>[1])
    expect(held.length).toBeLessThanOrEqual(splitIntoWords(' three').length)
  })
})

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
    await act(async () => {
      await new Promise((resolve) => requestAnimationFrame(() => resolve(null)))
    })

    const spans = document.querySelectorAll('.chat-fresh-text').length
    expect(spans).toBeLessThan(10)
    expect(editor.children.length).toBe(12)
  })
})

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

describe('a code fence streaming in', () => {
  it('keeps the lines it already drew and adds only the new one', () => {
    const fence = (lines: number) =>
      ['```ts', ...Array.from({ length: lines }, (_, i) => `const v${i} = ${i}`)].join('\n')
    const editor = createPlateEditor({
      plugins: chatComposerPlugins,
      value: chatMarkdownToValue(fence(3)),
    })
    const block = () => editor.children[0] as { children: unknown[] }
    const drawn = block().children.slice(0, 2)
    applyStreamedValue(editor, chatMarkdownToValue(fence(4)))
    expect(block().children.slice(0, 2)).toEqual(drawn)
    drawn.forEach((line, i) => expect(block().children[i]).toBe(line))
    expect(withoutIds(editor.children)).toEqual(withoutIds(chatMarkdownToValue(fence(4))))
  })
})
