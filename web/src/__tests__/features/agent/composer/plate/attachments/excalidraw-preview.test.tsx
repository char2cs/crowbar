import { act } from 'react'
import { render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { ExcalidrawPreview } from '@/features/agent/composer/plate/attachments/excalidraw-preview'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'

beforeEach(() => {
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
})

const scene: ParsedExcalidrawScene = {
  elements: [{ type: 'rectangle' }, { type: 'ellipse' }],
  appState: {},
}

const oneElementScene: ParsedExcalidrawScene = { elements: [{ type: 'rectangle' }], appState: {} }

/** Lets a test control exactly when `resolve()` settles for a given `src`,
 *  to exercise the effect's cancel-on-change / cancel-on-unmount guard —
 *  the real `ChatMarkdownAssetProvider` + fetch stub (used below for the
 *  end-to-end resolution test) settles a microtask too fast to interleave a
 *  rerender or unmount in between. */
function deferredAssetResolver() {
  const pending = new Map<string, (data: string | null) => void>()
  const asset = {
    wsId: 'ws1',
    fileDir: '',
    resolve: (src: string) =>
      new Promise<string | null>((resolve) => {
        pending.set(src, resolve)
      }),
  }
  return {
    asset,
    settle(src: string, data: string | null) {
      const resolve = pending.get(src)
      if (!resolve) throw new Error(`no pending resolve() call for ${src}`)
      pending.delete(src)
      resolve(data)
    },
  }
}

describe('ExcalidrawPreview', () => {
  it('shows a placeholder with the element count while there is no PNG ref', () => {
    render(<ExcalidrawPreview scene={scene} />)
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
    expect(screen.getByText(/2 elements/i)).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('singularizes the element count for exactly one element', () => {
    render(<ExcalidrawPreview scene={oneElementScene} />)
    expect(screen.getByText(/1 element\b/i)).toBeInTheDocument()
    expect(screen.queryByText(/1 elements/i)).not.toBeInTheDocument()
  })

  it('shows a placeholder when a pngRef is given but there is no asset context', () => {
    render(<ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />)
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
    expect(screen.getByText(/2 elements/i)).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('shows a placeholder when there is an asset context but no pngRef', () => {
    const { asset } = deferredAssetResolver()
    render(
      <MarkdownAssetContext.Provider value={asset}>
        <ExcalidrawPreview scene={scene} />
      </MarkdownAssetContext.Provider>,
    )
    expect(screen.getByText(/2 elements/i)).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('resolves and shows the persisted PNG once a pngRef and asset context are present', async () => {
    // A real `new Response(blob)` is avoided here: this repo's jsdom test
    // environment does not round-trip a jsdom `Blob` through Node/undici's
    // `Response` body handling (it silently stringifies the Blob instead of
    // reading its bytes) — see chat-asset-resolver.test.ts for the same note.
    // A minimal fetch-result stub with a real `blob()` resolver exercises the
    // same `response.ok` / `await response.blob()` path without hitting that
    // mismatch.
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )

    render(
      <ChatMarkdownAssetProvider wsId="ws1">
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: /excalidraw diagram/i })
    await waitFor(() => expect(img.getAttribute('src')).toMatch(/^data:image\/png;base64,/))
    expect(screen.queryByText(/2 elements/i)).not.toBeInTheDocument()
    vi.unstubAllGlobals()
  })

  it('cancels a stale resolution when pngRef changes before it settles', async () => {
    const { asset, settle } = deferredAssetResolver()
    const { rerender } = render(
      <MarkdownAssetContext.Provider value={asset}>
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/a.png" />
      </MarkdownAssetContext.Provider>,
    )

    rerender(
      <MarkdownAssetContext.Provider value={asset}>
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/b.png" />
      </MarkdownAssetContext.Provider>,
    )

    // The fresh ('b') resolution settles first...
    await act(async () => {
      settle('chats/c1/attachments/b.png', 'data:image/png;base64,Qg==')
    })
    const img = await screen.findByRole('img', { name: /excalidraw diagram/i })
    expect(img.getAttribute('src')).toBe('data:image/png;base64,Qg==')

    // ...and the stale ('a') resolution settling afterwards must not clobber it.
    await act(async () => {
      settle('chats/c1/attachments/a.png', 'data:image/png;base64,QQ==')
    })
    expect(screen.getByRole('img', { name: /excalidraw diagram/i }).getAttribute('src')).toBe(
      'data:image/png;base64,Qg==',
    )
  })

  it('does not update state after unmount when a pending resolution settles late', async () => {
    const { asset, settle } = deferredAssetResolver()
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    const { unmount } = render(
      <MarkdownAssetContext.Provider value={asset}>
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/a.png" />
      </MarkdownAssetContext.Provider>,
    )

    unmount()

    await act(async () => {
      settle('chats/c1/attachments/a.png', 'data:image/png;base64,QQ==')
    })

    expect(consoleError).not.toHaveBeenCalled()
    consoleError.mockRestore()
  })
})
