import { act, renderHook } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  hasPendingImageUpload,
  usePromptQueue,
  type PromptQueueOptions,
} from '@/features/agent/hooks/use-prompt-queue'

const submitAgentPrompt = vi.fn()
const getPendingPrompt = vi.fn()

vi.mock('@/features/agent/api/agent-api', () => ({
  submitAgentPrompt: (...args: unknown[]) => submitAgentPrompt(...args),
  getPendingPrompt: (...args: unknown[]) => getPendingPrompt(...args),
}))

// REGRESSION, reported live: "photos attachments are not loaded instantly...
// let's not wait for them." Once a photo shows an instant local preview
// (insertPendingImageInto/settlePendingImageInto, chat-markdown-editor.tsx)
// instead of waiting for its upload, sending has to wait for the SWAP
// instead — a `blob:` url is meaningless outside this browser session, and
// the agent could never fetch it. `enqueue` (usePromptQueue) refuses to
// queue a draft that still contains one; this is that check's own pure
// predicate, unit-tested directly the same way `isPromptTextWithinLimit`
// (prompt-queue-persistence.ts) already is.
describe('hasPendingImageUpload', () => {
  it('is true for a draft containing a pending image placeholder', () => {
    expect(hasPendingImageUpload('check this out ![photo.png](blob:local-preview-id) thanks')).toBe(
      true,
    )
  })

  it('is false for a draft with no images at all', () => {
    expect(hasPendingImageUpload('just some plain text')).toBe(false)
  })

  it('is false for a draft whose image already resolved to a real ref', () => {
    expect(hasPendingImageUpload('![photo.png](chats/c1/attachments/x-photo.png)')).toBe(false)
  })

  // The check is markdown-image-shaped (`![alt](blob:...)`), not a bare
  // substring match — plain text that happens to mention "blob" must not
  // false-positive and block sending.
  it('is false for text that merely mentions the word "blob"', () => {
    expect(hasPendingImageUpload('a blob: of clay, not an attachment')).toBe(false)
  })

  it('is true for multiple images even when only one is still pending', () => {
    const draft = '![done](chats/c1/attachments/a.png) and ![pending](blob:local-preview-id)'
    expect(hasPendingImageUpload(draft)).toBe(true)
  })
})

/** A promise whose settlement THIS test decides, so a dispatch can be observed
 *  in flight without a single timer. */
function deferred<T>() {
  let settle!: (value: T) => void
  const promise = new Promise<T>((resolve) => {
    settle = resolve
  })
  return { promise, settle }
}

function options(overrides: Partial<PromptQueueOptions> = {}): PromptQueueOptions {
  return {
    wsId: 'ws1',
    chatId: 'c1',
    working: false,
    compacting: false,
    live: true,
    active: true,
    visible: true,
    turnRevision: 0,
    terminalWaiting: false,
    getBaseline: () => 0,
    refreshMessages: () => {},
    onPromptSpawned: () => {},
    onRefreshChat: () => Promise.resolve(false),
    onSubmitUnavailable: () => {},
    ...overrides,
  }
}

function mount(initialProps: PromptQueueOptions) {
  return renderHook((props: PromptQueueOptions) => usePromptQueue(props), { initialProps })
}

// REGRESSION, reported live: "when compacting, a queued message gets sent right
// away, which stops the compacting action."
//
// The composer promises this in so many words — resolveComposerState's kind 7 is
// "compacting — busy; prompts queue", and the placeholder reads "Compacting…
// your message will be queued". The FIFO dispatcher did not honour it, because
// it was never told: `usePromptQueue` was handed `working` alone, and a bare
// /compact opens no tracked turn (see AgentChatsState.compacting), so the
// aggregate folds the entire compaction as IDLE. The head passed the busy guard
// on the very next render and went to the CLI mid-compaction, aborting it.
//
// Nothing here is timing-based: the dispatch promise is settled by hand, and
// every busy edge is an explicit rerender.
describe('usePromptQueue during a compaction', () => {
  beforeEach(() => {
    submitAgentPrompt.mockReset()
    submitAgentPrompt.mockResolvedValue({ runnerId: 'r1' })
    getPendingPrompt.mockReset()
    getPendingPrompt.mockResolvedValue(null)
    localStorage.clear()
  })

  it('holds a prompt typed mid-compaction instead of sending it', async () => {
    const { result } = mount(options({ compacting: true }))

    await act(async () => {
      result.current.enqueue('summarise the plan')
    })

    expect(submitAgentPrompt).not.toHaveBeenCalled()
    expect(result.current.queue.map((item) => item.state)).toEqual(['queued'])
  })

  // The flush-time half of the same check. This prompt was queued legitimately
  // (the chat was working), so the fix cannot be "refuse at enqueue": the busy
  // edge that normally releases it lands while a compaction is running, and the
  // dispatcher has to re-read BOTH busy signals at that moment.
  it('keeps holding when the working edge lands during a compaction', async () => {
    const { result, rerender } = mount(options({ working: true }))

    await act(async () => {
      result.current.enqueue('summarise the plan')
    })
    expect(submitAgentPrompt).not.toHaveBeenCalled()

    await act(async () => {
      rerender(options({ working: false, compacting: true }))
    })

    expect(submitAgentPrompt).not.toHaveBeenCalled()
    expect(result.current.queue.map((item) => item.state)).toEqual(['queued'])
  })

  it('dispatches the held prompt once the compaction clears', async () => {
    const delivery = deferred<{ runnerId: string }>()
    submitAgentPrompt.mockReturnValue(delivery.promise)
    const { result, rerender } = mount(options({ compacting: true }))

    await act(async () => {
      result.current.enqueue('summarise the plan')
    })
    expect(submitAgentPrompt).not.toHaveBeenCalled()

    await act(async () => {
      rerender(options({ compacting: false }))
    })

    expect(submitAgentPrompt).toHaveBeenCalledTimes(1)
    expect(submitAgentPrompt.mock.calls[0]?.[2]).toBe('summarise the plan')
    // In flight, and provably so: this test still owns the promise.
    expect(result.current.queue[0]?.state).toBe('submitting')

    await act(async () => {
      delivery.settle({ runnerId: 'r1' })
      await delivery.promise
    })

    expect(result.current.queue[0]?.state).toBe('awaiting_turn')
  })

  // Holding must not reorder the conversation, and must not release the whole
  // queue at once when it lifts — only the head can ever move.
  it('releases the held prompts in order, head first', async () => {
    const delivery = deferred<{ runnerId: string }>()
    submitAgentPrompt.mockReturnValue(delivery.promise)
    const { result, rerender } = mount(options({ compacting: true }))

    await act(async () => {
      result.current.enqueue('first')
      result.current.enqueue('second')
    })
    expect(submitAgentPrompt).not.toHaveBeenCalled()

    await act(async () => {
      rerender(options({ compacting: false }))
    })

    expect(submitAgentPrompt).toHaveBeenCalledTimes(1)
    expect(submitAgentPrompt.mock.calls[0]?.[2]).toBe('first')
    expect(result.current.queue.map((item) => item.state)).toEqual(['submitting', 'queued'])
  })
})

// REGRESSION, reported live against codex: "User's turns after some time of idle
// is lost, and does not record anywhere."
//
// The daemon retires a delivery that produced no turn and announces it. That
// announcement used to say only "this is over", and the queue answered by
// deleting the item — which deletes the user's TEXT, from this queue and, on the
// same tick, from localStorage. At that moment the queued item is the only copy
// of what the user typed anywhere in the system: the daemon's delivery journal
// records a hash of the prompt and never the text, and by definition nothing
// reached the ledger. The words were unrecoverable, and no error was shown.
//
// The frame now distinguishes the two cases. Nothing here is timing-based.
describe('usePromptQueue when the daemon retires a delivery', () => {
  beforeEach(() => {
    submitAgentPrompt.mockReset()
    submitAgentPrompt.mockResolvedValue({ runnerId: 'r1' })
    getPendingPrompt.mockReset()
    getPendingPrompt.mockResolvedValue(null)
    localStorage.clear()
  })

  /** Drives one prompt to `awaiting_turn` — the state a delivered prompt sits in
   *  while it waits for its user message to appear in the ledger. */
  async function awaitingPrompt() {
    const mounted = mount(options())
    await act(async () => {
      mounted.result.current.enqueue('the precious words the user typed')
    })
    await act(async () => {})
    expect(mounted.result.current.queue[0]?.state).toBe('awaiting_turn')
    return mounted
  }

  it('keeps the text when nothing proved the provider took the prompt', async () => {
    const { result, rerender } = await awaitingPrompt()
    const settledId = result.current.queue[0]?.clientRequestId ?? ''

    await act(async () => {
      rerender(options({ abandonedPrompts: [settledId] }))
    })

    expect(result.current.queue).toHaveLength(1)
    expect(result.current.queue[0]?.text).toBe('the precious words the user typed')
    expect(result.current.queue[0]?.state).toBe('failed')
    expect(result.current.queue[0]?.error).toBeTruthy()
  })

  // The words must survive a remount too: the queue is rewritten to localStorage
  // on the same tick it changes, so dropping the item there is exactly as
  // destructive as dropping it from React state.
  it('leaves the abandoned prompt on disk for a later mount to restore', async () => {
    const { result, rerender } = await awaitingPrompt()
    const settledId = result.current.queue[0]?.clientRequestId ?? ''

    await act(async () => {
      rerender(options({ abandonedPrompts: [settledId] }))
    })

    const persisted = localStorage.getItem('crowbar:agent-prompt-queue:v1:ws1:c1')
    expect(persisted).toContain('the precious words the user typed')
  })

  // The other half, and the reason the queue drops anything at all: a provider
  // built-in the CLI demonstrably ran announces no turn by design, so its text is
  // genuinely spent and a kept row would block the composer for good.
  it('drops the item when the daemon vouches that the provider consumed it', async () => {
    const { result, rerender } = await awaitingPrompt()
    const settledId = result.current.queue[0]?.clientRequestId ?? ''

    await act(async () => {
      rerender(options({ settledPrompts: [settledId] }))
    })

    expect(result.current.queue).toHaveLength(0)
  })
})

// REGRESSION, reported live against codex: "User's turns after some time of idle
// is lost, and does not record anywhere." A crashed tab, a cleared localStorage or
// an idle reload can wipe this queue's own record of a prompt that the backend
// still has outstanding. On becoming visible the queue asks the backend directly
// and recovers anything it has no record of, as an ordinary outcome_uncertain row
// with the same retry/edit affordances any other unconfirmed prompt gets.
describe('usePromptQueue recovering a lost prompt from the backend', () => {
  beforeEach(() => {
    submitAgentPrompt.mockReset()
    submitAgentPrompt.mockResolvedValue({ runnerId: 'r1' })
    getPendingPrompt.mockReset()
    getPendingPrompt.mockResolvedValue(null)
    localStorage.clear()
  })

  it('recovers a lost queued prompt from the backend when the chat becomes visible', async () => {
    getPendingPrompt.mockResolvedValueOnce({
      text: 'please rename this function',
      state: 'dispatching',
    })

    const { result } = mount(options({ visible: true }))
    await act(async () => {})

    expect(result.current.queue).toContainEqual(
      expect.objectContaining({
        text: 'please rename this function',
        state: 'outcome_uncertain',
      }),
    )
  })

  it('does not recover anything when the backend has nothing pending', async () => {
    getPendingPrompt.mockResolvedValueOnce(null)

    const { result } = mount(options({ visible: true }))
    await act(async () => {})

    expect(getPendingPrompt).toHaveBeenCalled()
    expect(result.current.queue).toEqual([])
  })

  // The dedup check must hold for a chat that already has its own record of the
  // same prompt — not just on the very first read. A second mount (a fresh tab,
  // or the same tab after a reload) re-reads the backend and must not duplicate
  // a row this tab's own persisted queue already carries.
  it('does not duplicate a prompt this tab already has a queued record of', async () => {
    const seeded = mount(options({ visible: true }))
    await act(async () => {
      seeded.result.current.enqueue('already tracked prompt')
    })
    await act(async () => {})
    seeded.unmount()

    getPendingPrompt.mockResolvedValueOnce({
      text: 'already tracked prompt',
      state: 'dispatching',
    })

    const { result } = mount(options({ visible: true }))
    await act(async () => {})

    expect(
      result.current.queue.filter((item) => item.text === 'already tracked prompt'),
    ).toHaveLength(1)
  })
})
