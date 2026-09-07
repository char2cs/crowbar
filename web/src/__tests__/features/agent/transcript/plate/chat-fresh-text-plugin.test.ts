import { describe, expect, it } from 'vitest'
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
