import { act, type ReactElement } from 'react'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { ExcalidrawPreview } from '@/features/agent/composer/plate/attachments/excalidraw-preview'
import { ChatMarkdownAssetProvider } from '@/features/agent/composer/plate/attachments/chat-markdown-asset-provider'
import { ChatIdContext } from '@/features/agent/composer/plate/attachments/chat-id-context'
import { MarkdownAssetContext } from '@/features/editor/markdown/plate/markdown-asset'
import { recordWorkspaceScope } from '@/lib/workspace-scope'
import type { ParsedExcalidrawScene } from '@/features/agent/composer/plate/attachments/excalidraw-scene'
import { useExcalidrawEditRequests } from '@/features/agent/composer/lib/excalidraw-edit-requests'
import { useWorkspaceScopeStore } from '@/lib/workspace-scope-store'

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
  useWorkspaceScopeStore.setState(useWorkspaceScopeStore.getInitialState())
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

function deferredAssetResolver() {
  const pending = new Map<string, (data: string | null) => void>()
  const asset = {
    wsId: 'ws1',
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
    await waitFor(() => expect(exportToSvgMock).toHaveBeenCalled())
    expect(screen.getByText(/excalidraw diagram/i)).toBeInTheDocument()
  })

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
    expect(img.style.filter).toBe('')
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

    await act(async () => {
      settle('chats/c1/attachments/b.png', 'data:image/png;base64,Qg==')
    })
    const img = await screen.findByRole('img', { name: /excalidraw diagram/i })
    expect(img.getAttribute('src')).toBe('data:image/png;base64,Qg==')

    await act(async () => {
      settle('chats/c1/attachments/a.png', 'data:image/png;base64,QQ==')
    })
    expect(screen.getByRole('img', { name: /excalidraw diagram/i }).getAttribute('src')).toBe(
      'data:image/png;base64,Qg==',
    )
  })

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
})

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

  it('applies the same dark-mode filter to a live SVG render', async () => {
    document.documentElement.classList.add('dark')
    exportToSvgMock.mockResolvedValue(fakeSvg())
    render(<ExcalidrawPreview scene={scene} />)

    await waitFor(() => expect(screen.getByTestId('live-excalidraw-render')).toBeInTheDocument())
    const host = screen.getByTestId('live-excalidraw-render').parentElement as HTMLElement
    expect(host.style.filter).toBe('invert(93%) hue-rotate(180deg)')
  })
})

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

describe('ExcalidrawPreview reservation follows a resize while still loading', () => {
  const tallScene: ParsedExcalidrawScene = {
    elements: [{ type: 'rectangle', x: 0, y: 0, width: 400, height: 200 }],
    appState: {},
  }

  let observerCallbacks: Array<() => void> = []
  const RealResizeObserver = globalThis.ResizeObserver

  beforeEach(() => {
    observerCallbacks = []
    class ControllableResizeObserver {
      callback: () => void
      constructor(callback: () => void) {
        this.callback = callback
      }
      observe() {
        observerCallbacks.push(this.callback)
      }
      unobserve() {}
      disconnect() {
        observerCallbacks = observerCallbacks.filter((c) => c !== this.callback)
      }
    }
    Object.defineProperty(globalThis, 'ResizeObserver', {
      value: ControllableResizeObserver,
      configurable: true,
      writable: true,
    })
  })

  afterEach(() => {
    Object.defineProperty(globalThis, 'ResizeObserver', {
      value: RealResizeObserver,
      configurable: true,
      writable: true,
    })
  })

  it('recomputes the reserved height after the container resizes, before the live render ever resolves', () => {
    let width = 400
    Object.defineProperty(HTMLElement.prototype, 'clientWidth', {
      configurable: true,
      get() {
        return this.classList.contains('excalidraw-preview') ? width : 0
      },
    })

    const { container } = render(<ExcalidrawPreview scene={tallScene} />)
    const box = container.querySelector('.excalidraw-preview') as HTMLElement

    expect(box.style.minHeight).toBe('200px')

    width = 200
    act(() => {
      for (const cb of observerCallbacks) cb()
    })

    expect(box.style.minHeight).toBe('100px')

    Reflect.deleteProperty(HTMLElement.prototype, 'clientWidth')
  })
})

describe('ExcalidrawPreview edit button', () => {
  const requested = (chatId: string) => useExcalidrawEditRequests.getState().requests[chatId]
  beforeEach(() => useExcalidrawEditRequests.setState({ requests: {} }))

  function renderWithChat(chatId: string | null, ui: ReactElement) {
    render(<ChatIdContext.Provider value={chatId}>{ui}</ChatIdContext.Provider>)
  }

  it('requests opening the takeover with this scene when Edit is clicked', () => {
    renderWithChat('c1', <ExcalidrawPreview scene={scene} />)

    fireEvent.click(screen.getByRole('button', { name: /edit/i }))

    expect(requested('c1')).toEqual(scene)
  })

  it('shows Edit on the persisted-PNG path too, not only the live-render path', async () => {
    const blob = new Blob(['x'], { type: 'image/png' })
    vi.stubGlobal(
      'fetch',
      vi.fn().mockResolvedValue({ ok: true, blob: () => Promise.resolve(blob) }),
    )
    render(
      <ChatIdContext.Provider value="c1">
        <ChatMarkdownAssetProvider wsId="ws1" chatId="c1">
          <ExcalidrawPreview scene={scene} pngRef="chats/c1/attachments/diagram.png" />
        </ChatMarkdownAssetProvider>
      </ChatIdContext.Provider>,
    )
    await screen.findByRole('img', { name: /excalidraw diagram/i })

    fireEvent.click(screen.getByRole('button', { name: /edit/i }))

    expect(requested('c1')).toEqual(scene)
    vi.unstubAllGlobals()
  })

  it('renders no Edit button outside a chat context', () => {
    render(<ExcalidrawPreview scene={scene} />)
    expect(screen.queryByRole('button', { name: /edit/i })).not.toBeInTheDocument()
  })

  it('is an icon button with no "Edit" text, floating with no wrap box around the diagram', () => {
    renderWithChat('c1', <ExcalidrawPreview scene={scene} />)
    const button = screen.getByRole('button', { name: /edit/i })
    expect(button.textContent).toBe('')
    expect(button.querySelector('svg')).not.toBeNull()
    expect(document.querySelector('.excalidraw-preview')?.className).not.toMatch(/\bborder\b/)
    expect(document.querySelector('.excalidraw-preview')?.className).not.toContain('bg-background')
  })
})
