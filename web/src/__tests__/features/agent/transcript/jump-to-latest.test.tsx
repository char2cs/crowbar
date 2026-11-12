import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useMobileScreenStore } from '@/features/window/stores/mobile-screen-store'
import type { NativeChromeIntent } from '@/lib/native-chrome'

const chrome = vi.hoisted(() => ({
  native: false,
  push: vi.fn(),
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
  intent: (intent: NativeChromeIntent) => chrome.listeners.forEach((listener) => listener(intent)),
}))
vi.mock('@/lib/native-chrome', () => ({
  hasNativeChrome: () => chrome.native,
  setNativeJumpToLatest: chrome.push,
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))

import { JumpToLatest } from '@/features/agent/transcript/jump-to-latest'

beforeEach(() => {
  chrome.native = false
  chrome.push.mockClear()
})
afterEach(cleanup)

describe('JumpToLatest on the web', () => {
  it('is an arrow-only button that still names itself', () => {
    const onJump = vi.fn()
    render(<JumpToLatest onJump={onJump} dockTop={92} />)
    const button = screen.getByRole('button', { name: 'Jump to latest' })
    expect(button.textContent).toBe('')
    fireEvent.click(button)
    expect(onJump).toHaveBeenCalledOnce()
  })
})

describe('JumpToLatest in the phone chrome', () => {
  beforeEach(() => {
    chrome.native = true
  })

  it('raises the chrome button above the dock, until its owner goes', () => {
    const { rerender, unmount } = render(<JumpToLatest onJump={() => {}} dockTop={92} />)
    const raised = chrome.push.mock.calls.at(-1)?.[0]
    expect(raised).toMatchObject({ visible: true, bottom: 104, label: 'Jump to latest' })
    rerender(<JumpToLatest onJump={() => {}} dockTop={140} />)
    expect(chrome.push).toHaveBeenLastCalledWith({ ...raised, bottom: 152 })
    render(<JumpToLatest onJump={() => {}} dockTop={92} />)
    expect(chrome.push.mock.calls.at(-1)?.[0].owner).not.toBe(raised.owner)
    unmount()
    expect(chrome.push).toHaveBeenLastCalledWith(
      expect.objectContaining({ visible: false, owner: raised.owner }),
    )
  })

  it('goes down under the IDE and returns with the chat, whose tap jumps its own', () => {
    const onJump = vi.fn()
    render(<JumpToLatest onJump={onJump} dockTop={92} />)
    act(() => useMobileScreenStore.getState().open('ide'))
    expect(chrome.push).toHaveBeenLastCalledWith(expect.objectContaining({ visible: false }))
    act(() => useMobileScreenStore.getState().back())
    expect(chrome.push).toHaveBeenLastCalledWith(expect.objectContaining({ visible: true }))
    const { owner } = chrome.push.mock.lastCall![0] as { owner: string }
    const splitPane = vi.fn()
    render(<JumpToLatest onJump={splitPane} dockTop={92} />)
    act(() => chrome.intent({ type: 'jumpToLatest', owner }))
    expect(onJump).toHaveBeenCalledOnce()
    expect(splitPane).not.toHaveBeenCalled()
  })
})
