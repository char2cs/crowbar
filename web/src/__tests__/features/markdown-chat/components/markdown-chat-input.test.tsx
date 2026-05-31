import { render } from '@testing-library/react'
import { vi } from 'vitest'

vi.mock('@excalidraw/excalidraw', () => ({ Excalidraw: () => null }))
vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn().mockResolvedValue({ svg: '<svg/>' }),
  },
}))

// Minimal CM6 mock — just enough for the component to mount without layout engine
vi.mock('@codemirror/view', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@codemirror/view')>()
  const RealEditorView = actual.EditorView

  class MockEditorView {
    state = {
      doc: {
        toString: () => '',
        length: 0,
        lines: 0,
        line: () => ({ text: '', from: 0, to: 0, number: 1 }),
        lineAt: () => ({ text: '', from: 0, to: 0, number: 1 }),
      },
      selection: { main: { head: 0, from: 0, to: 0 } },
      field: () => [],
    }
    dispatch = vi.fn()
    destroy = vi.fn()
    coordsAtPos = vi.fn(() => null)
    constructor({ parent }: { parent: Element }) {
      const div = document.createElement('div')
      div.className = 'cm-editor'
      parent.appendChild(div)
    }
  }

  // Copy static properties so extensions don't crash at import time
  const staticKeys = Object.getOwnPropertyNames(RealEditorView).filter(
    (k) => !['length', 'name', 'prototype'].includes(k),
  )
  for (const key of staticKeys) {
    Object.defineProperty(MockEditorView, key, Object.getOwnPropertyDescriptor(RealEditorView, key)!)
  }

  return { ...actual, EditorView: MockEditorView }
})

const { MarkdownChatInput } = await import('@/features/markdown-chat/components/markdown-chat-input')

test('renders without crashing', () => {
  const { container } = render(
    <MarkdownChatInput
      getTurns={() => []}
      onSubmit={vi.fn()}
      onWidgetChange={vi.fn()}
      onEditorReady={vi.fn()}
    />
  )
  expect(container.firstChild).toBeTruthy()
})

test('mounts CM6 editor into container', () => {
  const { container } = render(
    <MarkdownChatInput
      getTurns={() => []}
      onSubmit={vi.fn()}
      onWidgetChange={vi.fn()}
      onEditorReady={vi.fn()}
    />
  )
  expect(container.querySelector('.cm-editor')).toBeTruthy()
})
