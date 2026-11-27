import { fireEvent, render, screen } from '@testing-library/react'
import { StrictMode } from 'react'
import { beforeAll, beforeEach, describe, expect, it } from 'vitest'
import { MONACO_LOAD_MS } from '@/__tests__/__fixtures__/documents'
import { getEditorHost, loadEditorHost } from '@/features/editor/host/editor-host'
import { MarkdownEditorPane } from '@/features/editor/markdown/plate/markdown-editor-pane'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { resetWindowPaneStoreForTests } from '@/__tests__/__fixtures__/window-pane-store'
import type { EditorContent } from '@/features/panes/types/pane-content'
import { workspaceRef } from '@/lib/api/documents'

const BODY = Array.from({ length: 6 }, (_, i) => `## Section ${i}\n\nBody ${i}.\n`).join('\n')

const BUFFER_CONTENT: Record<string, string> = {
  b1: `# Doc one\n\n${BODY}`,
  b2: `# Doc two\n\n${BODY}`,
}

function openFixtures(): void {
  resetWindowPaneStoreForTests()
  windowPaneStore.setState({
    buffers: Object.entries(BUFFER_CONTENT).map(([id, text]): EditorContent => {
      getEditorHost()!.documents.open(workspaceRef('ws-1', `${id}.md`), 'markdown', text)
      return {
        id,
        type: 'editor',
        authority: 'workspace',
        workspaceId: 'ws-1',
        path: `${id}.md`,
        name: `${id}.md`,
        isDirty: false,
        isVirtual: false,
      }
    }),
  })
}

const renderPane = (bufferId: string) =>
  render(
    <StrictMode>
      <MarkdownEditorPane paneId="p1" bufferId={bufferId} />
    </StrictMode>,
  )

const scrollerOf = (view: ReturnType<typeof render>) =>
  view.container.querySelector('.crowbar-markdown-editor') as HTMLElement

beforeAll(() => loadEditorHost(), MONACO_LOAD_MS)

describe('MarkdownEditorPane scroll retention', () => {
  beforeEach(() => {
    openFixtures()
  })

  it('restores the scroll offset when the tab is switched away and back', async () => {
    const first = renderPane('b1')
    await screen.findByText('Doc one')

    const scroller = scrollerOf(first)
    scroller.scrollTop = 640
    fireEvent.scroll(scroller)

    first.unmount()

    const second = renderPane('b1')
    await screen.findByText('Doc one')
    expect(scrollerOf(second).scrollTop).toBe(640)
  })

  it('keeps each markdown buffer at its own offset', async () => {
    const first = renderPane('b1')
    await screen.findByText('Doc one')

    const scroller = scrollerOf(first)
    scroller.scrollTop = 640
    fireEvent.scroll(scroller)
    first.unmount()

    const other = renderPane('b2')
    await screen.findByText('Doc two')
    expect(scrollerOf(other).scrollTop).toBe(0)
  })
})
