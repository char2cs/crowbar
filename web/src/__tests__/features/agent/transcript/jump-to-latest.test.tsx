import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent } from '@/lib/native-chrome'

const chrome = vi.hoisted(() => ({
  native: false,
  push: vi.fn(),
  intent: null as ((intent: NativeChromeIntent) => void) | null,
}))
vi.mock('@/lib/native-chrome', () => ({
  hasNativeChrome: () => chrome.native,
  setNativeJumpToLatest: chrome.push,
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.intent = cb
    return () => {
      chrome.intent = null
    }
  },
}))

import { JumpToLatest } from '@/features/agent/transcript/jump-to-latest'

beforeEach(() => {
  chrome.native = false
  chrome.push.mockClear()
  chrome.intent = null
})
afterEach(cleanup)

describe('JumpToLatest on the web', () => {
  it('is an arrow-only button that still names itself', () => {
    const onJump = vi.fn()
    render(<JumpToLatest onJump={onJump} dockTop={92} />)
    const button = screen.getByRole('button', { name: 'Jump to latest' })
    expect(button.textContent).toBe('')
    expect(button.querySelector('svg')).not.toBeNull()
    fireEvent.click(button)
    expect(onJump).toHaveBeenCalledOnce()
    expect(chrome.push).not.toHaveBeenCalled()
  })
})

describe('JumpToLatest in the phone chrome', () => {
  beforeEach(() => {
    chrome.native = true
  })

  it('draws nothing itself and asks the chrome to show its button above the dock', () => {
    const { container } = render(<JumpToLatest onJump={() => {}} dockTop={92} />)
    expect(container.innerHTML).toBe('')
    expect(chrome.push).toHaveBeenLastCalledWith({
      visible: true,
      bottom: 104,
      label: 'Jump to latest',
    })
  })

  it('follows the dock as it grows', () => {
    const { rerender } = render(<JumpToLatest onJump={() => {}} dockTop={92} />)
    rerender(<JumpToLatest onJump={() => {}} dockTop={140} />)
    expect(chrome.push).toHaveBeenLastCalledWith(expect.objectContaining({ bottom: 152 }))
  })

  it('jumps when the chrome reports its tap, and ignores other intents', () => {
    const onJump = vi.fn()
    render(<JumpToLatest onJump={onJump} dockTop={92} />)
    act(() => chrome.intent?.({ type: 'closePlusMenu' }))
    expect(onJump).not.toHaveBeenCalled()
    act(() => chrome.intent?.({ type: 'jumpToLatest' }))
    expect(onJump).toHaveBeenCalledOnce()
  })

  it('asks for the button to go away once the reader is back at the bottom', () => {
    const { unmount } = render(<JumpToLatest onJump={() => {}} dockTop={92} />)
    unmount()
    expect(chrome.push).toHaveBeenLastCalledWith(expect.objectContaining({ visible: false }))
    expect(chrome.intent).toBeNull()
  })
})
