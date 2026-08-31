import { render } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import type { AgentChatMessage } from '@/features/agent/api/agent-api'
import { AgentTranscript } from '@/features/agent/transcript/agent-transcript'

/**
 * Scale gate for Task 5's virtualization of the historical transcript rows,
 * backing docs/superpowers/sdd/2026-08-30-transcript-static-render-and-virtualization.
 *
 * Before virtualization, `AgentTranscript` rendered one row per loaded message —
 * DOM cost O(messages) whether or not the user could see them. This test proves
 * the target behaviour: mounted row count is bounded by the virtual window, not
 * by the loaded message count. A regression to eager rendering fails here.
 *
 * DOM node count rather than wall time deliberately — it is deterministic, so
 * this is a real gate instead of a flaky timing assertion.
 */

// jsdom has no layout engine: every element measures 0×0, and a virtualiser
// told its viewport is zero pixels tall windows down to NOTHING —
// `calculateRange` bails on `outerSize === 0` before overscan is ever applied,
// so not one row mounts. Give elements a pane-sized rect before render so the
// window under test is a bounded one, the same problem
// changed-files-tree.scale.test.tsx and agent-transcript.test.tsx solve the
// same way. Purely geometric — no timers, no polling.
//
// Unlike ChangedFilesTree, AgentTranscript's virtualizer measures EACH row's
// own height via `measureElement: (el) => el.getBoundingClientRect().height`
// (agent-transcript.tsx:267), not just the scroll container's. A single
// blanket override (every element reports the full 800px viewport) makes
// every row measure as tall as the viewport itself, which is not what a real
// browser would lay out and isn't what determines the mounted count here.
// So the scroll container (`.scroll`, the element `observeScrollRect` reads)
// gets the viewport-sized rect, and every other element — the rows —
// gets a fixed stand-in row height instead: not a real content-based
// measurement (jsdom cannot produce one), but at least a row that is not the
// entire viewport, so the windowing this test is gating on reflects genuine
// per-row measurement rather than one row that happens to fill the screen.
const VIEWPORT_WIDTH = 768
const VIEWPORT_HEIGHT = 800
const ROW_HEIGHT = 64
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
  const viewportRect = {
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: VIEWPORT_HEIGHT,
    width: VIEWPORT_WIDTH,
    height: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
  }
  const rowRect = {
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: ROW_HEIGHT,
    width: VIEWPORT_WIDTH,
    height: ROW_HEIGHT,
    x: 0,
    y: 0,
  }
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect(
    this: HTMLElement,
  ) {
    const rect = this.classList.contains('scroll') ? viewportRect : rowRect
    return { ...rect, toJSON: () => rect } as DOMRect
  }
})

afterEach(() => {
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect
})

// Same shape as agent-transcript.test.tsx's own `conversation` fixture builder:
// alternating user/assistant turns, each a complete AgentChatMessage.
function messagesOfLength(n: number): AgentChatMessage[] {
  return Array.from({ length: n }, (_, i) => ({
    turnId: `t${i}`,
    sequence: i,
    role: i % 2 === 0 ? ('user' as const) : ('assistant' as const),
    providerId: i % 2 === 0 ? '' : 'claude',
    text: `message number ${i}`,
    at: '',
  }))
}

function draw(messages: AgentChatMessage[]) {
  return render(
    <AgentTranscript
      messages={messages}
      queue={[]}
      providers={[]}
      activity={{ toolCalls: [], subagents: [], interruptions: [], choices: [] }}
      working={false}
      loading={false}
      error={null}
      hasOlder={false}
      onLoadOlder={() => {}}
      onRetryLoad={() => {}}
      onOpenTerminal={() => {}}
      onEditPrompt={() => {}}
      onCancelPrompt={() => {}}
      onRetryPrompt={() => {}}
    />,
  )
}

function countMountedRows(container: HTMLElement) {
  return container.querySelectorAll('.virtual-rows > [data-index]').length
}

// Counting `[data-index]` wrappers alone would still pass if the virtualizer
// were changed to mount empty placeholder rows instead of real content — the
// "rows carry real MessageRow content" guarantee otherwise lives only in
// agent-transcript.test.tsx's own windowed-history tests, a different file.
// Every message here is `role: 'user' | 'assistant'`, so a genuinely rendered
// row always carries a `MessageRow` with this testid (message-row.tsx:131).
function countMountedMessageRows(container: HTMLElement) {
  return container.querySelectorAll('[data-testid^="agent-message-"]').length
}

describe('AgentTranscript scale', () => {
  it('keeps mounted row count roughly constant as loaded message count grows', () => {
    const small = draw(messagesOfLength(100))
    const smallCount = countMountedRows(small.container)

    const large = draw(messagesOfLength(2000))
    const largeCount = countMountedRows(large.container)

    // A scale test should report its measurement, not just gate on it.
    console.info(
      `[scale] 100 messages -> ${smallCount} rows, 2000 messages -> ${largeCount} rows ` +
        `(ratio ${(largeCount / smallCount).toFixed(2)})`,
    )

    expect(smallCount).toBeGreaterThan(0)
    expect(largeCount).toBeGreaterThan(0)

    // 20x the messages, the same viewport window: mounted count must not scale
    // with the input. Was exactly 20.0 before virtualization.
    expect(largeCount / smallCount).toBeLessThan(1.5)

    // The bounded count above is only the target behaviour if what's mounted
    // is real content, not empty placeholder wrappers — verify that here
    // rather than inferring it from a sibling test file.
    expect(countMountedMessageRows(small.container)).toBeGreaterThan(0)
    expect(countMountedMessageRows(large.container)).toBeGreaterThan(0)
  })
})
