import { createElement, createRef } from 'react'
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import {
  AgentEmptyDocument,
  type AgentEmptyDocumentHandle,
} from '@/features/agent/chat/agent-empty-document'

// Same stand-in agent-chat-view.test.tsx uses: jsdom never delivers a keydown
// to a real Slate editable, so these tests are not about the editor's own
// behaviour (verified live and in its own suite) — only about the handle ref
// this file adds.
vi.mock('@/features/agent/composer/plate/chat-markdown-editor', () => ({
  ChatMarkdownEditor: () => createElement('div', { 'data-testid': 'editor-stub' }),
}))

function draw(overrides: Partial<Parameters<typeof AgentEmptyDocument>[0]> = {}) {
  const ref = createRef<AgentEmptyDocumentHandle>()
  const view = render(
    <AgentEmptyDocument
      ref={ref}
      draft=""
      draftSeed={0}
      onDraftChange={vi.fn()}
      onSubmit={vi.fn()}
      onKeyDown={vi.fn()}
      controls={null}
      working={false}
      canStop={false}
      sending={false}
      onStop={vi.fn()}
      {...overrides}
    />,
  )
  return { ...view, ref }
}

describe('AgentEmptyDocument handle', () => {
  it('reports the handle’s own on-screen rect', () => {
    const { ref, container } = draw()

    const handle = container.querySelector('.dochandle') as HTMLElement
    const rect = { top: 84, left: 0, right: 0, bottom: 0, width: 0, height: 0 } as DOMRect
    vi.spyOn(handle, 'getBoundingClientRect').mockReturnValue(rect)

    expect(ref.current?.getHandleRect()).toBe(rect)
  })

  // A caller reading this off an unmounted instance gets "nothing to arrive
  // from", never a thrown error or a stale rect.
  it('reports null once unmounted', () => {
    const { ref, unmount } = draw()
    unmount()

    expect(ref.current).toBeNull()
  })
})

describe('AgentEmptyDocument stop control', () => {
  // REGRESSION: this surface hand-duplicates composer-handle.tsx's own
  // send/stop button, and used to gate `stopping` on the document being
  // empty the same way — hiding the only way to interrupt a turn already
  // running (a background handoff can start one before anything is typed)
  // the instant a person started writing.
  it('stops a running turn even with text already in the document', () => {
    const onStop = vi.fn()
    draw({ draft: 'a follow-up thought', working: true, canStop: true, onStop })

    const button = screen.getByRole('button', { name: 'Stop this turn' })
    expect(button).toBeEnabled()
    expect(button.className).toMatch(/\bhalt\b/)
    fireEvent.click(button)
    expect(onStop).toHaveBeenCalled()
  })

  it('sends the document when there is no running turn to stop', () => {
    const onSubmit = vi.fn()
    draw({ draft: 'the whole plan', onSubmit })

    const button = screen.getByRole('button', { name: 'Send prompt' })
    expect(button).toBeEnabled()
    fireEvent.click(button)
    expect(onSubmit).toHaveBeenCalled()
  })
})

// REGRESSION: this surface's send button hand-duplicates composer-handle.tsx's
// own, but had no `sending` state at all — the FIRST message in a chat clears
// the document and shows nothing while its own dispatch is in flight, where
// every later message (composer-handle.tsx) gets a spinner. Live-verified via
// a MutationObserver on the real button: the dock's spinner appears and
// disappears cleanly; this surface's button never once changed class before
// being replaced.
describe('AgentEmptyDocument sending state', () => {
  it('shows a sending spinner once dispatched but not yet proven delivered, same as the dock composer', () => {
    const { container } = draw({ sending: true })

    const button = screen.getByRole('button', { name: 'Sending' })
    expect(button).toBeDisabled()
    expect(button.className).toMatch(/\boff\b/)
    expect(container.querySelector('[data-flicker-spinner]')).toBeInTheDocument()
  })

  it('prefers the stop control over the sending spinner when both apply', () => {
    draw({ working: true, canStop: true, sending: true })

    expect(screen.getByRole('button', { name: 'Stop this turn' })).toBeInTheDocument()
    expect(screen.queryByRole('status')).toBeNull()
  })
})
