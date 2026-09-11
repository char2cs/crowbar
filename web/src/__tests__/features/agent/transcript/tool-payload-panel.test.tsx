import { act, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ToolPayloadPanel } from '@/features/agent/transcript/tool-payload-panel'

const getToolPayloadFn = vi.hoisted(() => vi.fn())

vi.mock('@/features/agent/api/agent-api', () => ({
  getPendingPrompt: vi.fn().mockResolvedValue(null),
  getToolPayload: (...args: unknown[]) => getToolPayloadFn(...args),
}))

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>((res) => {
    resolve = res
  })
  return { promise, resolve }
}

describe('ToolPayloadPanel', () => {
  beforeEach(() => {
    getToolPayloadFn.mockReset()
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  // Regression: react-doctor flagged the effect's own catch block writing
  // `setSides` unconditionally, including for a request the effect's OWN
  // cleanup aborted (a different tool row expanded). Two overlapping reads
  // share `sides` state keyed only by label ('Request'/'Result'), not by
  // toolId — a stale rejection resolving AFTER a fresher read has already
  // started for the same label could clobber that fresh read's own state.
  it('does not let a stale, aborted read for a previous toolId clobber a fresher read for the same label', async () => {
    const first = deferred<string | null>()
    const second = deferred<string | null>()
    getToolPayloadFn.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise)

    const { rerender } = render(
      <ToolPayloadPanel wsId="w1" chatId="c1" toolId="tool-a" hasRequest hasResult={false} />,
    )
    expect(screen.getByText('Loading…')).toBeInTheDocument()

    // A different row expands before the first read resolves — the effect's
    // cleanup aborts the first read's controller and a second read starts.
    rerender(
      <ToolPayloadPanel wsId="w1" chatId="c1" toolId="tool-b" hasRequest hasResult={false} />,
    )

    // The stale first read resolves late, after the second has already
    // started — it must not overwrite the panel back into a loading/failed
    // state for the label the second read now owns.
    await act(async () => {
      first.resolve('stale payload from tool-a')
      await Promise.resolve()
      await Promise.resolve()
    })
    expect(screen.getByText('Loading…')).toBeInTheDocument()
    expect(screen.queryByText('stale payload from tool-a')).not.toBeInTheDocument()

    await act(async () => {
      second.resolve('real payload from tool-b')
    })
    await screen.findByText('real payload from tool-b')
  })

  it('shows a genuinely failed read as no longer available, not silently forever loading', async () => {
    getToolPayloadFn.mockRejectedValueOnce(new Error('gone'))

    render(<ToolPayloadPanel wsId="w1" chatId="c1" toolId="tool-a" hasRequest hasResult={false} />)

    await screen.findByText('No longer available')
  })
})
