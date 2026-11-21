import { afterEach, describe, expect, it } from 'vitest'
import { followChatZoom } from '@/entries/follow-chat-zoom'
import { useZoomStore } from '@/features/window/stores/zoom-store'

function shellThatSends(percent: number) {
  return { onChatZoom: (listener: (percent: number) => void) => (listener(percent), () => {}) }
}

afterEach(() => useZoomStore.getState().actions.resetZoom())

describe('followChatZoom', () => {
  it.each([
    [120, 1.2],
    [30, 0.3],
    [100, 1],
  ])('shows the chat at the shell window’s %i%%', (percent, zoom) => {
    followChatZoom(shellThatSends(percent))
    expect(useZoomStore.getState().zoom).toBe(zoom)
  })
})
