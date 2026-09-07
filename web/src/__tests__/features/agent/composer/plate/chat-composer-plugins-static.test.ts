import { describe, expect, it } from 'vitest'

import {
  chatComposerPlugins,
  chatComposerPluginsStatic,
} from '@/features/agent/composer/plate/chat-composer-plugins'

// `dnd` (`@platejs/dnd`'s `DndPlugin`, wired in by Task 35) is dropped from
// the static derivation for the same reason `chat-floating-toolbar` is: a
// settled message is read, not reordered, and neither static node component
// (`ChatCodeBlockElementStatic`/the file card's static variant) ever calls
// `useAttachmentDraggable`, so nothing there would read `editor.plugins.dnd`
// in the first place — see chat-composer-plugins.ts's `STATIC_EXCLUDED_KEYS`.
const DROPPED_KEYS = new Set(['chat-floating-toolbar', 'dnd'])

describe('chatComposerPluginsStatic', () => {
  it('has the same plugin keys as chatComposerPlugins, minus the floating toolbar and dnd, in order', () => {
    expect(chatComposerPluginsStatic.map((p) => p.key)).toEqual(
      chatComposerPlugins.map((p) => p.key).filter((key) => !DROPPED_KEYS.has(key)),
    )
  })

  // `code_block` is included alongside link/callout: `ChatCodeBlockElementStatic`
  // (Task 35) is a genuinely different component from the interactive
  // `ChatCodeBlockElement` — same reason link/callout swap — even though
  // `inputRules`/`shortcuts` are unchanged between the two. `p` and `img`
  // joined once ordinary paragraphs became drop targets and images became
  // draggable, respectively (both interactive-only) — `p` appears TWICE
  // because `chatComposerPlugins` itself registers the paragraph plugin
  // twice (once via `BasicNodesKit`, once via the explicit override that
  // replaces it — see chat-composer-plugins.ts), and the static derivation
  // swaps BOTH positions, each producing a new (by-reference) plugin object
  // even where the resulting component is the same.
  it('swaps only link, callout, code_block, paragraph and img; drops the floating toolbar and dnd rather than swapping them', () => {
    expect(chatComposerPluginsStatic.some((p) => DROPPED_KEYS.has(p.key))).toBe(false)
    const shared = chatComposerPluginsStatic.filter((p) => !DROPPED_KEYS.has(p.key))
    const interactiveWithoutDropped = chatComposerPlugins.filter((p) => !DROPPED_KEYS.has(p.key))
    const changed = shared.filter((plugin, i) => plugin !== interactiveWithoutDropped[i])
    expect(changed.map((p) => p.key).sort()).toEqual(
      ['a', 'callout', 'code_block', 'img', 'p', 'p'].sort(),
    )
  })
})
