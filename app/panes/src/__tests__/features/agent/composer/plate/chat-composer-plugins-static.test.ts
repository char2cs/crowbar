import { afterEach, describe, expect, it, vi } from 'vitest'
import { createPlateEditor } from 'platejs/react'

import {
  chatComposerPlugins,
  chatComposerPluginsStatic,
} from '@/features/agent/composer/plate/chat-composer-plugins'

const DROPPED_KEYS = new Set(['mark-floating-toolbar'])

describe('chatComposerPluginsStatic', () => {
  it('has the same plugin keys as chatComposerPlugins, minus the floating toolbar, in order', () => {
    expect(chatComposerPluginsStatic.map((p) => p.key)).toEqual(
      chatComposerPlugins.map((p) => p.key).filter((key) => !DROPPED_KEYS.has(key)),
    )
  })

  it('swaps only link, callout, code_block, code_syntax, paragraph and img; drops the floating toolbar and dnd rather than swapping them', () => {
    expect(chatComposerPluginsStatic.some((p) => DROPPED_KEYS.has(p.key))).toBe(false)
    const shared = chatComposerPluginsStatic.filter((p) => !DROPPED_KEYS.has(p.key))
    const interactiveWithoutDropped = chatComposerPlugins.filter((p) => !DROPPED_KEYS.has(p.key))
    const changed = shared.filter((plugin, i) => plugin !== interactiveWithoutDropped[i])
    expect(changed.map((p) => p.key).sort()).toEqual(
      ['a', 'callout', 'code_block', 'code_syntax', 'img', 'p', 'p'].sort(),
    )
  })
})

const line = (text: string) => ({ type: 'p', children: [{ text }] })

afterEach(() => vi.restoreAllMocks())

describe('chatComposerPlugins', () => {
  it('appends a block without serialising the document', () => {
    const editor = createPlateEditor({ plugins: chatComposerPlugins, value: [line('a')] })
    const stringify = vi.spyOn(JSON, 'stringify')
    editor.tf.insertNodes(line('b'), { at: [1] })
    expect(editor.children).toHaveLength(2)
    expect(stringify).not.toHaveBeenCalled()
  })

  it('still finds a node that is there and nothing where there is none', () => {
    const editor = createPlateEditor({ plugins: chatComposerPlugins, value: [line('a')] })
    expect(editor.api.node([0])?.[0]).toMatchObject(line('a'))
    expect(editor.api.node([3])).toBeUndefined()
    expect(editor.api.node([0, 4])).toBeUndefined()
  })
})
