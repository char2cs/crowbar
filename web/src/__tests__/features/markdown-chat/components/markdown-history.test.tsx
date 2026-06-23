import { render, act } from '@testing-library/react'
import { describe, test, expect, vi, beforeEach } from 'vitest'
import { useStore } from 'zustand'
import type { MarkdownTurn } from '@/features/markdown-chat/types'

// Spy on react-markdown so we can count how many times each turn's markdown is
// *parsed*. We only care about render frequency, not output, so the mock just
// records the call (keyed by content) and renders the raw text.
const parseCounts = new Map<string, number>()
vi.mock('react-markdown', () => ({
  default: ({ children }: { children: string }) => {
    parseCounts.set(children, (parseCounts.get(children) ?? 0) + 1)
    return <div data-md>{children}</div>
  },
}))

// Heavy lazy block renderers don't work in jsdom — stub them (mirrors the
// markdown-chat-view test).
vi.mock('@excalidraw/excalidraw', () => ({ Excalidraw: () => null }))
vi.mock('mermaid', () => ({
  default: { initialize: vi.fn(), render: vi.fn().mockResolvedValue({ svg: '<svg/>' }) },
}))

const { MarkdownHistory } = await import(
  '@/features/markdown-chat/components/markdown/markdown-history'
)
const { getOrCreateConversationStore, destroyConversationStore } = await import(
  '@/features/markdown-chat/stores/conversation-store'
)

const CHAT_ID = 'history-rerender-test'

beforeEach(() => {
  parseCounts.clear()
  destroyConversationStore(CHAT_ID)
})

function finalizedTurn(id: string, content: string): MarkdownTurn {
  return {
    id,
    role: 'agent',
    content,
    timestamp: '2026-06-20T00:00:00Z',
    authorName: 'Claude',
    widgets: [],
    streaming: false,
  }
}

// Thin reactive wrapper so the store drives the history exactly like the real
// MarkdownChatView (useStore selector on `turns`).
function StoreDrivenHistory() {
  const store = getOrCreateConversationStore(CHAT_ID)
  const turns = useStore(store, (s) => s.turns)
  // Stable no-op callback (real view passes a useCallback'd handler).
  return <MarkdownHistory turns={turns} onWidgetChange={onWidgetChange} />
}
const onWidgetChange = () => {}

describe('MarkdownHistory streaming re-render isolation (H12)', () => {
  test('updateStreamingTurn keeps prior turn object references stable', () => {
    const store = getOrCreateConversationStore(CHAT_ID)
    store.getState().appendTurn(finalizedTurn('done-1', '# Finalized'))
    store.getState().appendTurn({
      id: 'stream-1',
      role: 'agent',
      content: '',
      timestamp: '2026-06-20T00:00:01Z',
      authorName: 'Claude',
      widgets: [],
      streaming: true,
    })

    const before = store.getState().turns
    const finalizedBefore = before.find((t) => t.id === 'done-1')!

    store.getState().updateStreamingTurn('stream-1', 'hello')

    const after = store.getState().turns
    const finalizedAfter = after.find((t) => t.id === 'done-1')!
    // The array is new (immutable update)...
    expect(after).not.toBe(before)
    // ...but the untouched turn keeps its exact object reference.
    expect(finalizedAfter).toBe(finalizedBefore)
  })

  test('streaming a turn does NOT re-parse finalized turns', () => {
    const store = getOrCreateConversationStore(CHAT_ID)
    store.getState().appendTurn(finalizedTurn('done-1', '# Finalized'))
    // Streaming turns render plain text (no markdown parse), so to observe the
    // re-parse bug we add the streaming turn AFTER mount via finalize-then-stream
    // emulation: append a finalized turn, then a streaming one, then stream it.
    store.getState().appendTurn({
      id: 'stream-1',
      role: 'agent',
      content: 'partial',
      timestamp: '2026-06-20T00:00:01Z',
      authorName: 'Claude',
      widgets: [],
      streaming: false, // start finalized so it parses once...
    })

    render(<StoreDrivenHistory />)

    // Both turns parsed exactly once on mount.
    expect(parseCounts.get('# Finalized')).toBe(1)
    expect(parseCounts.get('partial')).toBe(1)

    // Now drive several streaming-style content updates on stream-1. Each fires
    // a new `turns` array → MarkdownChatView/History re-render, mimicking a WS
    // content frame.
    act(() => {
      for (const delta of ['-a', '-b', '-c', '-d']) {
        store.getState().updateStreamingTurn('stream-1', delta)
      }
    })

    // The finalized turn must NOT have been re-parsed — its memo'd TurnItem is
    // skipped because its object reference is unchanged.
    expect(parseCounts.get('# Finalized')).toBe(1)
    // The updated turn re-parses (its content/reference changed each frame).
    expect(parseCounts.get('partial-a-b-c-d')).toBe(1)
  })

  test('memoized TurnItem skips re-render for an unchanged turn reference', () => {
    const turns = [finalizedTurn('done-1', 'alpha'), finalizedTurn('done-2', 'beta')]
    const { rerender } = render(
      <MarkdownHistory turns={turns} onWidgetChange={onWidgetChange} />,
    )
    expect(parseCounts.get('alpha')).toBe(1)
    expect(parseCounts.get('beta')).toBe(1)

    // New array, but every turn object + the callback are reference-stable.
    rerender(<MarkdownHistory turns={[...turns]} onWidgetChange={onWidgetChange} />)

    // Nothing re-parsed.
    expect(parseCounts.get('alpha')).toBe(1)
    expect(parseCounts.get('beta')).toBe(1)
  })
})
