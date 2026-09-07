import { act, type ReactElement } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ExcalidrawPreview } from '@/features/agent/composer/plate/attachments/excalidraw-preview'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { ChatIdContext } from '@/features/agent/composer/plate/attachments/chat-id-context'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { __resetWorkspaceScopesForTest, recordWorkspaceScope } from '@/lib/workspace-scope'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'
import { createWorkspaceStore } from '@/features/workspace/stores/workspace-store'
import { WorkspaceStoreContext } from '@/features/workspace/stores/workspace-context'

// The real `@excalidraw/excalidraw` needs Canvas/font-metrics APIs jsdom
// doesn't implement — mocked so the live-render path is deterministic and
// fast here, not dependent on how far jsdom happens to fake a canvas.
// Default (no test overrides it): exportToSvg rejects, so every test not
// specifically about the live-render path keeps seeing the placeholder,
// exactly as before this path existed.
const restoreMock = vi.hoisted(() =>
  vi.fn((data: { elements: unknown; appState: unknown }) => ({
    elements: data.elements,
    appState: data.appState,
    files: {},
  })),
)
const exportToSvgMock = vi.hoisted(() => vi.fn())
vi.mock('@excalidraw/excalidraw', () => ({ restore: restoreMock, exportToSvg: exportToSvgMock }))

function fakeSvg(): SVGSVGElement {
  const svg = document.createElementNS('http://www.w3.org/2000/svg', 'svg')
  svg.setAttribute('data-testid', 'live-excalidraw-render')
  return svg
}

beforeEach(() => {
  __resetWorkspaceScopesForTest()
  recordWorkspaceScope({ projectId: 'p1', repoId: 'r1', wsId: 'ws1' })
  restoreMock.mockClear()
  exportToSvgMock.mockReset().mockRejectedValue(new Error('jsdom cannot render this'))
})

afterEach(() => {
  document.documentElement.classList.remove('dark')
})

const scene: ParsedExcalidrawScene = {
  elements: [{ type: 'rectangle' }, { type: 'ellipse' }],
  appState: {},
}

const oneElementScene: ParsedExcalidrawScene = { elements: [{ type: 'rectangle' }], appState: {} }

/** Lets a test control exactly when `resolve()` settles for a given `src`,
 *  to exercise the effect's cancel-on-dependency-change guard — the real
 *  `ChatMarkdownAssetProvider` + fetch stub (used below for the end-to-end
 *  resolution test) settles a microtask too fast to interleave a rerender
 *  in between. */
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
  it('shows a placeholder with the element count when there is no PNG ref and the live render fails', async () => {
    render(<ExcalidrawPreview scene={scene} />)
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
    expect(screen.getByText(/2 elements/i)).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
    // exportToSvgMock rejects by default (beforeEach) — let that settle and
    // confirm the placeholder is still what's shown, not an empty box.
    await waitFor(() => expect(exportToSvgMock).toHaveBeenCalled())
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
  })

  // REGRESSION: this is the actual fix — an agent-authored fence has no
  // persisted PNG (nothing ever ran the editor to export one), so without a
  // live render it only ever showed this placeholder text, never the
  // diagram itself.
  it('renders the scene live via exportToSvg when there is no PNG ref', async () => {
    exportToSvgMock.mockResolvedValue(fakeSvg())
    render(<ExcalidrawPreview scene={scene} />)

    await waitFor(() => expect(screen.getByTestId('live-excalidraw-render')).toBeInTheDocument())
    expect(screen.queryByText(/excalidraw diagram/i)).not.toBeInTheDocument()
    expect(restoreMock).toHaveBeenCalledWith(
      { elements: scene.elements, appState: scene.appState, files: undefined },
      null,
      null,
    )
  })

  // REGRESSION, reported live: the previous approach mounted the exported
  // svg via `hostRef.current.replaceChildren(svg)` — a DOM mutation outside
  // React's own rendering — which, since this node sits inside a Slate VOID
  // element, desynced slate-react's DOM<->node mapping ("Cannot resolve a
  // DOM node from Slate node", logged live) and broke the composer's own
  // height-tracking downstream. Rendering through `dangerouslySetInnerHTML`
  // keeps this subtree inside React's reconciliation on every update,
  // including a scene change swapping the rendered diagram.
  it('replaces the rendered svg when the scene changes, leaving exactly one in the DOM', async () => {
    exportToSvgMock.mockResolvedValueOnce(fakeSvg())
    const { rerender } = render(<ExcalidrawPreview scene={scene} />)
    await waitFor(() => expect(screen.getByTestId('live-excalidraw-render')).toBeInTheDocument())

    exportToSvgMock.mockResolvedValueOnce(fakeSvg())
    rerender(<ExcalidrawPreview scene={oneElementScene} />)
    await waitFor(() => expect(exportToSvgMock).toHaveBeenCalledTimes(2))

    expect(screen.getAllByTestId('live-excalidraw-render')).toHaveLength(1)
  })

  it('never calls exportToSvg when a pngRef is present — the persisted PNG wins', () => {
    render(<ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />)
    expect(exportToSvgMock).not.toHaveBeenCalled()
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
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
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

  // REGRESSION: exportToSvg renders whatever `scene.elements`/`appState` say
  // — content an agent or another chat participant can fully control (it's
  // just attachment JSON) — and the result went straight into
  // `dangerouslySetInnerHTML` with no sanitization. A `<script>` survives
  // `.outerHTML` serialization even though the SVG spec doesn't otherwise
  // execute it there; DOMPurify strips it, matching every other
  // dangerouslySetInnerHTML sink in this codebase.
  it('sanitizes the live SVG render, stripping a script element the scene JSON could smuggle in', async () => {
    const svg = fakeSvg()
    const script = document.createElementNS('http://www.w3.org/2000/svg', 'script')
    script.textContent = 'window.pwned = true'
    svg.appendChild(script)
    exportToSvgMock.mockResolvedValue(svg)

    render(<ExcalidrawPreview scene={scene} />)

    await waitFor(() => expect(screen.getByTestId('live-excalidraw-render')).toBeInTheDocument())
    expect(document.querySelector('script')).not.toBeInTheDocument()
  })

  // Unmount safety is deliberately NOT covered by a separate test. React
  // invokes the exact same effect-cleanup closure (`cancelled = true`) on
  // unmount as it does before re-running the effect for a dependency change
  // — there is no distinct "unmount branch" in the implementation, so the
  // test above already exercises the guard that protects both triggers.
  // A black-box unmount test can't independently discriminate guarded from
  // unguarded behaviour here either way: React 19 removed the "state update
  // on an unmounted component" warning, and a setState call on an already-
  // unmounted fiber is a silent no-op with no rerender, no throw, and no
  // console output regardless of whether `cancelled` was set — so a test
  // that unmounts, settles the stale promise, and asserts "nothing observable
  // happened" would pass identically with the guard deleted.
})

// REGRESSION: neither the persisted PNG nor a live SVG render ever changed
// color with the app's theme — both are `@excalidraw/excalidraw` exports,
// and dark mode there is a CSS filter on its own <canvas>, never baked into
// what exportToSvg/exportToBlob produce. Reproducing that same filter here
// is what actually fixes it, for a PNG saved under either theme too.
describe('ExcalidrawPreview dark mode', () => {
  it('applies the excalidraw dark-mode filter to the persisted PNG when the app is in dark mode', async () => {
    document.documentElement.classList.add('dark')
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )

    render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: /excalidraw diagram/i })
    expect(img.style.filter).toBe('invert(93%) hue-rotate(180deg)')
    vi.unstubAllGlobals()
  })

  it('applies no filter to the persisted PNG in light mode', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )

    render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: /excalidraw diagram/i })
    expect(img.style.filter).toBe('')
    vi.unstubAllGlobals()
  })

  it('applies the same dark-mode filter to a live SVG render', async () => {
    document.documentElement.classList.add('dark')
    exportToSvgMock.mockResolvedValue(fakeSvg())
    render(<ExcalidrawPreview scene={scene} />)

    await waitFor(() => expect(screen.getByTestId('live-excalidraw-render')).toBeInTheDocument())
    const host = screen.getByTestId('live-excalidraw-render').parentElement as HTMLElement
    expect(host.style.filter).toBe('invert(93%) hue-rotate(180deg)')
  })
})

// REGRESSION, reported live: a prompted or agent-output diagram took as much
// vertical space in the transcript as its own content demanded — unbounded,
// unlike every other attachment kind. A max-height (with the aspect ratio
// preserved, not cropped) caps it the same way for both render paths.
describe('ExcalidrawPreview height cap', () => {
  it('caps the persisted PNG at a maximum height', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )

    render(
      <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
        <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />
      </ChatMarkdownAssetProvider>,
    )

    const img = await screen.findByRole('img', { name: /excalidraw diagram/i })
    expect(img.className).toContain('max-h-80')
    expect(img.className).toContain('object-contain')
    vi.unstubAllGlobals()
  })

  it('caps a live SVG render at the same maximum height', async () => {
    exportToSvgMock.mockResolvedValue(fakeSvg())
    render(<ExcalidrawPreview scene={scene} />)

    await waitFor(() => expect(screen.getByTestId('live-excalidraw-render')).toBeInTheDocument())
    const host = screen.getByTestId('live-excalidraw-render').parentElement as HTMLElement
    expect(host.className).toContain('max-h-80')
  })
})

// REGRESSION: an agent's own diagram, or the user's past one, could only ever
// be looked at in the transcript — this is what actually lets someone iterate
// on a diagram that already exists rather than always starting a fresh one.
describe('ExcalidrawPreview edit button', () => {
  function renderWithChat(chatId: string | null, ui: ReactElement) {
    const store = createWorkspaceStore('ws1')
    render(
      <WorkspaceStoreContext.Provider value={store}>
        <ChatIdContext.Provider value={chatId}>{ui}</ChatIdContext.Provider>
      </WorkspaceStoreContext.Provider>,
    )
    return store
  }

  it('requests opening the takeover with this scene when Edit is clicked', () => {
    const store = renderWithChat('c1', <ExcalidrawPreview scene={scene} />)

    fireEvent.click(screen.getByRole('button', { name: /edit/i }))

    expect(store.getState().agentChats.excalidrawEditRequests['c1']).toEqual(scene)
  })

  it('shows Edit on the persisted-PNG path too, not only the live-render path', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )
    const store = createWorkspaceStore('ws1')
    render(
      <WorkspaceStoreContext.Provider value={store}>
        <ChatIdContext.Provider value="c1">
          <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
            <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />
          </ChatMarkdownAssetProvider>
        </ChatIdContext.Provider>
      </WorkspaceStoreContext.Provider>,
    )
    await screen.findByRole('img', { name: /excalidraw diagram/i })

    fireEvent.click(screen.getByRole('button', { name: /edit/i }))

    expect(store.getState().agentChats.excalidrawEditRequests['c1']).toEqual(scene)
    vi.unstubAllGlobals()
  })

  it('renders no Edit button outside a chat/workspace context', () => {
    render(<ExcalidrawPreview scene={scene} />)
    expect(screen.queryByRole('button', { name: /edit/i })).not.toBeInTheDocument()
  })

  it('renders no Edit button when there is a workspace store but no chat id', () => {
    renderWithChat(null, <ExcalidrawPreview scene={scene} />)
    expect(screen.queryByRole('button', { name: /edit/i })).not.toBeInTheDocument()
  })

  // REGRESSION, reported live: a bordered/backgrounded box around the whole
  // preview, plus a worded "Edit" button, was more chrome than the image
  // itself needed — an icon button floating directly over the diagram reads
  // the same way the drag/delete controls do on every other attachment kind.
  it('is an icon button with no "Edit" text, floating with no wrap box around the diagram', () => {
    renderWithChat('c1', <ExcalidrawPreview scene={scene} />)
    const button = screen.getByRole('button', { name: /edit/i })
    expect(button.textContent).toBe('')
    expect(button.querySelector('svg')).not.toBeNull()
    expect(document.querySelector('.excalidraw-preview')?.className).not.toMatch(/\bborder\b/)
    expect(document.querySelector('.excalidraw-preview')?.className).not.toContain('bg-background')
  })
})
