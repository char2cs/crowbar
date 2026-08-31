import { describe, expect, it } from 'vitest'

import {
  chatComposerPlugins,
  chatComposerPluginsStatic,
} from '@/features/agent/composer/plate/chat-composer-plugins'

describe('chatComposerPluginsStatic', () => {
  it('has the same plugin keys, in the same order, as chatComposerPlugins', () => {
    expect(chatComposerPluginsStatic.map((p) => p.key)).toEqual(
      chatComposerPlugins.map((p) => p.key),
    )
  })

  it('swaps only link and callout', () => {
    const changed = chatComposerPluginsStatic.filter(
      (plugin, i) => plugin !== chatComposerPlugins[i],
    )
    expect(changed.map((p) => p.key).sort()).toEqual(['a', 'callout'].sort())
  })
})
