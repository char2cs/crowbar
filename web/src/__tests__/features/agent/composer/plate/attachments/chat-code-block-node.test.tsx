import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import type { PlateEditor } from 'platejs/react'
import { MarkdownMessage } from '@/features/agent/transcript/plate/markdown-message'
import { MarkdownMessageStatic } from '@/features/agent/transcript/plate/markdown-message-static'
import { findFollowingImageRef } from '@/features/agent/composer/plate/attachments/chat-code-block-node'
import * as ExcalidrawPreviewModule from '@/features/agent/composer/plate/attachments/excalidraw-preview'

// Wraps the REAL component (not a stand-in) so every existing behavioural
// assertion below still exercises actual rendering — this only adds the
// ability to inspect the `pngRef` prop `ChatCodeBlockElement` computed for
// it, which two tests below need to prove the sibling-`img`-lookup branches
// (found vs. not-found) without duplicating ExcalidrawPreview's own
// pngRef-resolution tests (see excalidraw-preview.test.tsx).
vi.mock(
  '@/features/agent/composer/plate/attachments/excalidraw-preview',
  async (importOriginal) => {
    const actual =
      await importOriginal<
        typeof import('@/features/agent/composer/plate/attachments/excalidraw-preview')
      >()
    return { ...actual, ExcalidrawPreview: vi.fn(actual.ExcalidrawPreview) }
  },
)

describe('chat attachment code blocks', () => {
  it('renders a plain code block for a bare ```text-attachment fence with no id', () => {
    render(
      <MarkdownMessageStatic>
        {'```text-attachment\njust discussing the feature\n```'}
      </MarkdownMessageStatic>,
    )
    expect(screen.getByText('just discussing the feature')).toBeInTheDocument()
    expect(screen.queryByText(/pasted text/i)).toBeNull()
  })

  it('renders a plain code block for a completely unrelated language tag, identical to before', () => {
    render(<MarkdownMessageStatic>{'```go\nfunc main() {}\n```'}</MarkdownMessageStatic>)
    expect(document.querySelector('pre')).not.toBeNull()
    expect(document.querySelector('pre')?.textContent).toContain('func main')
    expect(screen.queryByText(/pasted text/i)).toBeNull()
    expect(screen.queryByText(/excalidraw diagram/i)).toBeNull()
  })

  it('renders a pill for a fence with a valid id, keeping the raw text mounted but hidden', () => {
    render(
      <MarkdownMessageStatic>
        {'```text-attachment:AbC123xy\nsome long pasted text\n```'}
      </MarkdownMessageStatic>,
    )
    expect(screen.getByRole('button', { name: /pasted text/i })).toBeInTheDocument()
    expect(screen.getByText('some long pasted text').closest('.hidden')).not.toBeNull()
  })

  it('renders a plain code block for an excalidraw fence with content that is not valid scene JSON', () => {
    render(<MarkdownMessageStatic>{'```excalidraw:AbC123xy\nnot json\n```'}</MarkdownMessageStatic>)
    expect(screen.getByText('not json')).toBeInTheDocument()
    expect(screen.queryByText(/excalidraw diagram/i)).toBeNull()
  })

  it('renders a diagram preview for a valid excalidraw fence', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    render(
      <MarkdownMessageStatic>{`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\``}</MarkdownMessageStatic>,
    )
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
    // The raw fenced JSON stays mounted (hidden), same as the text-attachment
    // case above — Slate's node<->DOM mapping must never be disturbed.
    expect(screen.getByText(scene).closest('.hidden')).not.toBeNull()
  })

  it("threads a following img node's url through to ExcalidrawPreview as pngRef", () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    render(<MarkdownMessageStatic>{md}</MarkdownMessageStatic>)

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: 'chats/c1/attachments/diagram.png' })
  })

  it('leaves pngRef undefined when the following node exists but is not an img', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\nSome other paragraph.`
    render(<MarkdownMessageStatic>{md}</MarkdownMessageStatic>)

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: undefined })
  })

  it('leaves pngRef undefined when there is no following node at all', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    render(
      <MarkdownMessageStatic>{`\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\``}</MarkdownMessageStatic>,
    )

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: undefined })
  })

  // Every test above renders through MarkdownMessageStatic. This is the one
  // exercising `MarkdownMessage`'s INTERACTIVE editor — the other surface
  // `ChatCodeBlockElement` is registered on (chatComposerPlugins, not just
  // its static derivative) — so the sibling-lookup logic is proven to run
  // there too, not only under static rendering. It does not, and cannot by
  // itself, prove immunity to `useNodePath`'s memoized-staleness caveat (see
  // `findFollowingImageRef`'s own doc comment and this file's bottom
  // describe block for why that specific scenario has no reliable black-box
  // reproduction); it establishes that the interactive path renders
  // correctly at all, which no other test here did before.
  it('threads pngRef correctly through the INTERACTIVE renderer too, not just its static derivative', () => {
    const scene = JSON.stringify({ elements: [{ type: 'rectangle' }], appState: {} })
    const md = `\`\`\`excalidraw:AbC123xy\n${scene}\n\`\`\`\n\n![diagram](chats/c1/attachments/diagram.png)`
    render(<MarkdownMessage>{md}</MarkdownMessage>)

    const spy = vi.mocked(ExcalidrawPreviewModule.ExcalidrawPreview)
    const lastCallProps = spy.mock.calls.at(-1)?.[0]
    expect(lastCallProps).toMatchObject({ pngRef: 'chats/c1/attachments/diagram.png' })
  })
})

/**
 * `findFollowingImageRef`'s own unit coverage for `editor.api.findPath`
 * returning `undefined` — the guard exists for `findPath`'s own `Path |
 * undefined` return contract (an element no longer in the tree), not
 * anything a mounted document can trigger, so it's tested directly rather
 * than through a full render.
 *
 * This function deliberately calls `editor.api.findPath(props.element)`
 * fresh rather than trusting `props.path`: in the interactive renderer,
 * `props.path` is sourced from Plate's `useNodePath`, memoized on
 * `[editor.api, node]` — its own JSDoc says it will NOT recompute when a
 * change elsewhere shifts this node's path without replacing the node
 * object. A black-box regression test for that exact scenario was attempted
 * (mount a real interactive editor via `chatComposerPlugins`, insert a
 * sibling before this block, assert the sibling-image lookup still finds the
 * right node) and dropped: it passed identically whether `findFollowingImageRef`
 * used `props.path` or `findPath`, because slate-react's own `MemoizedElement`
 * wrapper (`prev.element === next.element`, confirmed by reading
 * `node_modules/slate-react/dist/index.js`) skips re-rendering this node at
 * all unless its OWN `element` reference changes — which invalidates
 * `useNodePath`'s memo too, so by the time this component's render body ever
 * runs again, the path is never actually stale in practice. That is an
 * internal implementation detail of a dependency two layers down, not a
 * documented contract, so `findPath` is used anyway — it costs one extra
 * (rare-path) tree-walk and matches Plate's own stated `useNodePath` caveat
 * rather than depending on slate-react's current memoization strategy never
 * changing.
 */
describe('findFollowingImageRef', () => {
  it('returns undefined, without looking up a sibling, when the element has no path', () => {
    const nodeSpy = vi.fn()
    const fakeEditor = {
      api: { findPath: () => undefined, node: nodeSpy },
    } as unknown as PlateEditor
    const result = findFollowingImageRef({
      editor: fakeEditor,
      element: { type: 'code_block', children: [] },
    } as unknown as Parameters<typeof findFollowingImageRef>[0])

    expect(result).toBeUndefined()
    expect(nodeSpy).not.toHaveBeenCalled()
  })
})
