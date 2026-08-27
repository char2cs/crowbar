import { createElement, createRef } from 'react'
import { render } from '@testing-library/react'
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
