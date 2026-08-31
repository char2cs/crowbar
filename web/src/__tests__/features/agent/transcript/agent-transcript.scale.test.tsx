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
// window under test is the realistic one the app produces. Purely geometric —
// no timers, no polling. Same stub, same reason as agent-transcript.test.tsx
// and changed-files-tree.scale.test.tsx.
const VIEWPORT_WIDTH = 768
const VIEWPORT_HEIGHT = 800
const originalGetBoundingClientRect = HTMLElement.prototype.getBoundingClientRect

beforeEach(() => {
  const rect = {
    top: 0,
    left: 0,
    right: VIEWPORT_WIDTH,
    bottom: VIEWPORT_HEIGHT,
    width: VIEWPORT_WIDTH,
    height: VIEWPORT_HEIGHT,
    x: 0,
    y: 0,
  }
  HTMLElement.prototype.getBoundingClientRect = function getBoundingClientRect() {
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
  })
})
