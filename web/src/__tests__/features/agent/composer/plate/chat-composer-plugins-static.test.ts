import { describe, expect, it } from 'vitest'

import {
  chatComposerPlugins,
  chatComposerPluginsStatic,
} from '@/features/agent/composer/plate/chat-composer-plugins'

describe('chatComposerPluginsStatic', () => {
  it('has the same plugin keys as chatComposerPlugins, minus the floating toolbar, in order', () => {
    expect(chatComposerPluginsStatic.map((p) => p.key)).toEqual(
      chatComposerPlugins.map((p) => p.key).filter((key) => key !== 'chat-floating-toolbar'),
    )
  })

  it('swaps only link and callout; drops the floating toolbar rather than swapping it', () => {
    expect(chatComposerPluginsStatic.some((p) => p.key === 'chat-floating-toolbar')).toBe(false)
    const shared = chatComposerPluginsStatic.filter((p) => p.key !== 'chat-floating-toolbar')
    const interactiveWithoutToolbar = chatComposerPlugins.filter(
      (p) => p.key !== 'chat-floating-toolbar',
    )
    const changed = shared.filter((plugin, i) => plugin !== interactiveWithoutToolbar[i])
    expect(changed.map((p) => p.key).sort()).toEqual(['a', 'callout'].sort())
  })
})
