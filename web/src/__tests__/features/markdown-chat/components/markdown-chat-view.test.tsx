import { render, screen, waitFor } from '@testing-library/react'
import { vi, beforeEach, afterEach, beforeAll, afterAll } from 'vitest'
import { setupServer } from 'msw/node'
import { QueryClientProvider } from '@tanstack/react-query'
import { queryClient } from '@/lib/queries/client'
import { markdownChatHandlers } from '@/mocks/handlers/markdown-chat'
import { destroyConversationStore, getOrCreateConversationStore } from '@/features/markdown-chat/stores/conversation-store'

const server = setupServer(...markdownChatHandlers)
beforeAll(() => server.listen())
afterEach(() => server.resetHandlers())
afterAll(() => server.close())

// Mock heavy dependencies that don't work well in jsdom
vi.mock('@excalidraw/excalidraw', () => ({
  Excalidraw: () => null,
}))

vi.mock('mermaid', () => ({
  default: {
    initialize: vi.fn(),
    render: vi.fn().mockResolvedValue({ svg: '<svg/>' }),
  },
}))

// CM6 EditorView requires layout — provide a minimal stub.
// Static properties (decorations, theme, lineWrapping, etc.) are copied from the
// real EditorView so that top-level StateField.define calls in extensions still work.
vi.mock('@codemirror/view', async (importOriginal) => {
  const mod = await importOriginal<typeof import('@codemirror/view')>()
  const RealEditorView = mod.EditorView

  class MockEditorView {
    state: {
      doc: {
        toString: () => string
        length: number
        lines: number
        line: (n: number) => { text: string; from: number; to: number; number: number }
        lineAt: (pos: number) => { text: string; from: number; to: number; number: number }
      }
      selection: { main: { head: number; from: number; to: number } }
      field: () => unknown[]
    }
    dom: HTMLElement
    constructor({ parent }: { parent: Element; state: unknown }) {
      this.dom = document.createElement('div')
      this.dom.className = 'cm-editor'
      parent.appendChild(this.dom)
      this.state = {
        doc: {
          toString: () => '',
          length: 0,
          lines: 0,
          line: (_n: number) => ({ text: '', from: 0, to: 0, number: 1 }),
          lineAt: (_pos: number) => ({ text: '', from: 0, to: 0, number: 1 }),
        },
        selection: { main: { head: 0, from: 0, to: 0 } },
        field: () => [],
      }
    }
    dispatch = vi.fn()
    destroy = vi.fn()
    coordsAtPos = vi.fn(() => null)
  }

  // Copy all static properties from the real EditorView onto the mock class so
  // extensions that call EditorView.decorations / EditorView.theme at import time
  // don't crash with "Cannot read properties of undefined".
  const staticKeys = Object.getOwnPropertyNames(RealEditorView).filter(
    (k) => !['length', 'name', 'prototype'].includes(k),
  )
  for (const key of staticKeys) {
    Object.defineProperty(
      MockEditorView,
      key,
      Object.getOwnPropertyDescriptor(RealEditorView, key)!,
    )
  }

  return { ...mod, EditorView: MockEditorView }
})

// After mocking EditorView, import the component under test
const { MarkdownChatView } = await import('@/features/markdown-chat/components/markdown-chat-view')

const WS_ID = 'ws-smoke-test'
const STEP_ID = 'brainstorm'

beforeEach(() => {
  destroyConversationStore(WS_ID)
  queryClient.clear()
})

afterEach(() => {
  destroyConversationStore(WS_ID)
})

test('renders without crashing', async () => {
  const { container } = render(
    <QueryClientProvider client={queryClient}>
      <MarkdownChatView workspaceId={WS_ID} stepId={STEP_ID} />
    </QueryClientProvider>
  )
  // Either the loading state or the editor mounted
  await waitFor(() => {
    const hasEditor = container.querySelector('.cm-editor') !== null
    const hasLoading = screen.queryByText('Loading conversation…') !== null
    expect(hasEditor || hasLoading).toBe(true)
  })
})

test('starts empty (no pre-generated greeting) for an unknown workspace', async () => {
  const { container } = render(
    <QueryClientProvider client={queryClient}>
      <MarkdownChatView workspaceId={WS_ID} stepId={STEP_ID} />
    </QueryClientProvider>
  )
  // The editable input mounts and fills the canvas...
  await waitFor(() => {
    expect(container.querySelector('.cm-editor')).not.toBeNull()
  })
  // ...with no seeded turns.
  const { turns } = getOrCreateConversationStore(WS_ID).getState()
  expect(turns.length).toBe(0)
})

test('seeds mock turns for ws3 brainstorm step', async () => {
  destroyConversationStore('ws3')
  render(
    <QueryClientProvider client={queryClient}>
      <MarkdownChatView workspaceId="ws3" stepId="brainstorm" />
    </QueryClientProvider>
  )
  await waitFor(() => {
    const store = getOrCreateConversationStore('ws3')
    const { turns } = store.getState()
    expect(turns.length).toBeGreaterThan(0)
    // First turn is user turn from mock data
    expect(turns[0].role).toBe('user')
  })
  destroyConversationStore('ws3')
})
