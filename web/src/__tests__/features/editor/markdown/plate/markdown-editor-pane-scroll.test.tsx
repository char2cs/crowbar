import { fireEvent, render, screen } from '@testing-library/react'
import { StrictMode } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { clearPreservedScroll } from '@/features/editor/hooks/use-preserved-scroll'
import { MarkdownEditorPane } from '@/features/editor/markdown/plate/markdown-editor-pane'

// Deliberately small. jsdom has no layout engine, so the document's height is
// irrelevant here — `scrollTop` is a plain property that never clamps, and what
// this file asserts is that the offset SURVIVES the unmount. A realistically
// long document only makes Plate slow enough to trip the 5s test timeout under
// a loaded suite. Clamping against still-growing content is covered where it
// can actually be modelled, in use-preserved-scroll.test.tsx.
const BODY = Array.from({ length: 6 }, (_, i) => `## Section ${i}\n\nBody ${i}.\n`).join('\n')

const BUFFER_CONTENT: Record<string, string> = {
  b1: `# Doc one\n\n${BODY}`,
  b2: `# Doc two\n\n${BODY}`,
}

vi.mock('@/features/workspace/stores/hooks/use-buffer-store', () => ({
  useBufferById: (bufferId: string) => {
    const content = BUFFER_CONTENT[bufferId] ?? ''
    return {
      id: bufferId,
      type: 'editor',
      path: `/repo/${bufferId}.md`,
      name: `${bufferId}.md`,
      content,
      savedContent: content,
      isDirty: false,
    }
  },
}))
vi.mock('@/features/editor/stores/editor-app-store', () => ({
  useEditorAppStore: { use: { actions: () => ({ handleContentChange: vi.fn() }) } },
}))
vi.mock('@/features/window/stores/toast-store', () => ({ toast: { error: vi.fn() } }))

// StrictMode like the real app root: its double-invoked effects (setup →
// cleanup → setup) are what broke the first cut of the preview's retained
// scroll, by capturing a freshly mounted element's 0 over the stored offset.
const renderPane = (bufferId: string) =>
  render(
    <StrictMode>
      <MarkdownEditorPane paneId="p1" bufferId={bufferId} />
    </StrictMode>,
  )

const scrollerOf = (view: ReturnType<typeof render>) =>
  view.container.querySelector('.crowbar-markdown-editor') as HTMLElement

describe('MarkdownEditorPane scroll retention', () => {
  beforeEach(() => {
    clearPreservedScroll()
  })

  // Regression: switching the pane to another tab unmounts the rich editor
  // entirely (PaneContainer renders only the active buffer, and EditorPane keys
  // the Plate pane by buffer id), so the scroll offset — which lived only on
  // that DOM node — used to reset to the top on the way back.
  it('restores the scroll offset when the tab is switched away and back', async () => {
    const first = renderPane('b1')
    await screen.findByText('Doc one')

    const scroller = scrollerOf(first)
    scroller.scrollTop = 640
    fireEvent.scroll(scroller)

    // Tab switch: the pane stops rendering this buffer entirely.
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
