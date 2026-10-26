import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { NativeChromeIntent, NativeContext } from '@/lib/native-chrome'
import type { AgentTelemetry } from '@/features/agent/api/agent-api'

const chrome = vi.hoisted(() => ({
  pushed: [] as NativeContext[],
  listeners: new Set<(intent: NativeChromeIntent) => void>(),
}))
vi.mock('@/lib/native-chrome', () => ({
  setNativeContext: (context: NativeContext) => chrome.pushed.push(context),
  onNativeChromeIntent: (cb: (intent: NativeChromeIntent) => void) => {
    chrome.listeners.add(cb)
    return () => chrome.listeners.delete(cb)
  },
}))

const { useNativeContext } = await import('@/features/agent/hooks/use-native-context')

const HIDDEN = { percent: null, usedLabel: '', canCompact: false, compacting: false }
const reported: AgentTelemetry = {
  context: { usedPercent: 6.4, usedTokens: 12400, capacityTokens: 200000 },
} as AgentTelemetry
const send = (intent: NativeChromeIntent) =>
  act(() => chrome.listeners.forEach((listener) => listener(intent)))
const last = () => chrome.pushed.at(-1)!

type Props = Parameters<typeof useNativeContext>[0]
const mount = (props: Partial<Props> = {}) =>
  renderHook((p: Props) => useNativeContext(p), {
    initialProps: {
      visible: true,
      telemetry: reported,
      compacting: false,
      onCompact: undefined,
      ...props,
    },
  })

beforeEach(() => {
  chrome.pushed.length = 0
  chrome.listeners.clear()
})

describe('useNativeContext', () => {
  it('reports the visible chat’s usage, and hides the meter when it goes', () => {
    const { unmount } = mount()
    expect(last()).toEqual({
      percent: 6,
      usedLabel: '12,400 of 200,000 tokens',
      canCompact: false,
      compacting: false,
    })
    unmount()
    expect(last()).toEqual(HIDDEN)
  })

  it('shows no meter before the provider reports, rather than 0%', () => {
    mount({ telemetry: null })
    expect(last()).toEqual(HIDDEN)
  })

  it('says nothing for a chat that is not on screen', () => {
    mount({ visible: false })
    expect(chrome.pushed.every((c) => c.percent === null)).toBe(true)
    send({ type: 'compact' })
  })

  it('compacts when the meter asks and compaction is offered', () => {
    const onCompact = vi.fn()
    mount({ onCompact })
    expect(last().canCompact).toBe(true)
    send({ type: 'compact' })
    expect(onCompact).toHaveBeenCalledTimes(1)
  })

  it('says a compaction is running', () => {
    mount({ compacting: true })
    expect(last()).toMatchObject({ compacting: true, canCompact: false })
  })
})
