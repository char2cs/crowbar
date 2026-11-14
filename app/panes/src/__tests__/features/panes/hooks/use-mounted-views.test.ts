import { act, renderHook } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useMountedViews } from '@/features/panes/hooks/use-mounted-views'
import { pendingChatSurfaces } from '@/features/panes/stores/pending-chat-surfaces'
import { createLeaf } from '@/features/panes/utils/pane-layout'

const views = [
  { id: 'showing', showing: true, layout: createLeaf('p1') },
  { id: 'parked', showing: false, layout: createLeaf('p2') },
]
const ids = (mounted: { id: string }[]) => mounted.map((v) => v.id)

async function runIdleSlots() {
  await act(async () => {
    await vi.runAllTimersAsync()
  })
}

describe('useMountedViews', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    pendingChatSurfaces.setState({ pending: new Set() })
  })
  afterEach(() => vi.useRealTimers())

  it('mounts the showing view at once and a parked view in a later slot', async () => {
    const { result } = renderHook(() => useMountedViews(views))
    expect(ids(result.current)).toEqual(['showing'])

    await runIdleSlots()

    expect(ids(result.current)).toEqual(['showing', 'parked'])
  })

  it('holds parked views back while a chat surface is still waiting to commit', async () => {
    pendingChatSurfaces.getState().begin('p1')
    const { result } = renderHook(() => useMountedViews(views))

    await runIdleSlots()
    expect(ids(result.current)).toEqual(['showing'])

    await act(async () => pendingChatSurfaces.getState().end('p1'))
    await runIdleSlots()

    expect(ids(result.current)).toEqual(['showing', 'parked'])
  })
})
