import type React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

// @base-ui/react ships pure ESM and gets its own React copy in the
// vitest/jsdom process. Popover/Avatar are mocked to plain, always-"open"
// markup so this file can exercise IconPopover's own logic — which button
// calls onStage vs. the network — without fighting Base UI's real
// open/close mechanics.
vi.mock('@/components/ui/popover', () => ({
  Popover: ({ children }: { children: React.ReactNode }) => <>{children}</>,
  PopoverTrigger: ({ children, ...props }: React.ComponentProps<'button'>) => (
    <button {...props}>{children}</button>
  ),
  PopoverContent: ({ children, ...props }: React.ComponentProps<'div'>) => (
    <div {...props}>{children}</div>
  ),
}))
vi.mock('@/components/ui/avatar', () => ({
  Avatar: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  AvatarImage: (props: React.ComponentProps<'img'>) => <img alt="" {...props} />,
  AvatarFallback: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}))

const apiFetch = vi.fn().mockResolvedValue(undefined)
vi.mock('@/lib/api', () => ({ apiFetch: (...args: unknown[]) => apiFetch(...args) }))

const openDialog = vi.fn()
vi.mock('@/lib/native-dialog', () => ({
  openNativeDialog: (...args: unknown[]) => openDialog(...args),
}))

const isTauri = vi.fn(() => false)
vi.mock('@/lib/crowbar-bridge', () => ({
  isTauri: () => isTauri(),
  convertFileSrc: (path: string) => `asset://staged${path}`,
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

import { IconPopover } from '@/components/layout/icon-popover'
import { toast } from '@/features/window/stores/toast-store'
import { useHostConfigStore } from '@/features/hosts/stores/host-config-store'
import { pairRemoteHost, resetHostWorld } from '@/__tests__/__fixtures__/remote-host'

beforeEach(() => {
  vi.clearAllMocks()
  isTauri.mockReturnValue(false)
})

const baseProps = {
  name: 'my-project',
  fallback: <span>fallback</span>,
  fallbackLarge: <span>fallback-large</span>,
}

describe('IconPopover — real (non-staged) mode is unchanged', () => {
  it('emoji submit PUTs to base/icon/emoji', () => {
    render(<IconPopover {...baseProps} base="/v0/projects/p1" />)
    fireEvent.click(screen.getByRole('button', { name: 'Emoji' }))
    fireEvent.change(screen.getByPlaceholderText('Type an emoji…'), {
      target: { value: '🚀' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Set' }))

    expect(apiFetch).toHaveBeenCalledExactlyOnceWith(
      '/v0/projects/p1/icon/emoji',
      expect.objectContaining({ method: 'PUT', body: JSON.stringify({ emoji: '🚀' }) }),
    )
  })

  it('reset DELETEs base/icon', () => {
    render(<IconPopover {...baseProps} base="/v0/projects/p1" emoji="🚀" />)
    fireEvent.click(screen.getByRole('button', { name: 'Reset to default' }))

    expect(apiFetch).toHaveBeenCalledExactlyOnceWith('/v0/projects/p1/icon', { method: 'DELETE' })
  })

  // A caller may wrap this trigger in a bare, unstyled `<span>` — that span is
  // an inline formatting context, so the trigger's own `inline-flex` box is
  // subject to `vertical-align: baseline` there unless it opts out. Caught
  // live: an uploaded project/repo icon sitting a few px above its name.
  // `align-middle` is meaningless (and harmless) once a caller's wrapper is
  // itself flex/grid, so asserting its presence here can't regress a caller
  // that never needed it.
  it('the trigger opts out of inline baseline alignment for bare-span callers', () => {
    render(<IconPopover {...baseProps} base="/v0/projects/p1" />)
    expect(screen.getByRole('button', { name: 'Edit my-project icon' }).className).toContain(
      'align-middle',
    )
  })
})

// An icon upload sends a file from this machine, whichever daemon the entity lives on — it
// is never a path picked on the entity's own host.
describe('IconPopover — an upload is a file on this machine', () => {
  it("a remote entity's upload still picks an image with the OS dialog", async () => {
    isTauri.mockReturnValue(true)
    openDialog.mockResolvedValue(null)
    render(<IconPopover {...baseProps} base="crowbar://h1/v0/projects/p1" />)

    fireEvent.click(screen.getByRole('button', { name: 'Upload' }))

    await vi.waitFor(() =>
      expect(openDialog).toHaveBeenCalledExactlyOnceWith({
        multiple: false,
        filters: [{ name: 'Images', extensions: ['png', 'jpg', 'jpeg', 'webp'] }],
      }),
    )
    expect(screen.queryByRole('dialog', { name: /choose a folder/i })).not.toBeInTheDocument()
  })
})

// A remote daemon cannot read this computer's disk: it is sent the picked image's bytes, while
// the local daemon keeps reading the path itself.
describe('IconPopover — an upload to a remote entity sends the image, never its path', () => {
  const REMOTE = 'crowbar://h1/v0/projects/p1'
  const reads: string[] = []
  const sentTo = (prefix: string) =>
    vi.mocked(fetch).mock.calls.filter(([url]) => String(url).startsWith(prefix))

  /** The webview's asset protocol serving the picked image (`size` is what its HEAD reports). */
  function serveImage(bytes: string, size = bytes.length) {
    reads.length = 0
    vi.stubGlobal(
      'fetch',
      vi.fn(async (url: string, init?: RequestInit) => {
        reads.push(`${init?.method ?? 'GET'} ${url}`)
        if (init?.method === 'HEAD') {
          return new Response(null, { headers: { 'content-length': String(size) } })
        }
        return Object.assign(new Response(null), {
          blob: async () => new Blob([bytes], { type: 'image/png' }),
        })
      }),
    )
  }

  const upload = () => fireEvent.click(screen.getByRole('button', { name: 'Upload' }))

  beforeEach(() => {
    isTauri.mockReturnValue(true)
    openDialog.mockResolvedValue('/Users/me/icon.png')
    serveImage('pixels')
  })

  afterEach(() => {
    vi.unstubAllGlobals()
    resetHostWorld()
  })

  it('PUTs the image’s bytes as multipart, never its path on this computer', async () => {
    render(<IconPopover {...baseProps} base={REMOTE} />)
    upload()

    await vi.waitFor(() => expect(apiFetch).toHaveBeenCalledOnce())
    const [url, init] = apiFetch.mock.calls[0] as [string, RequestInit]
    expect(url).toBe(`${REMOTE}/icon`)
    expect(init.method).toBe('PUT')
    expect((init.headers as Record<string, string>)['Content-Type']).toMatch(
      /^multipart\/form-data; boundary=/,
    )
    const body = new TextDecoder().decode(init.body as Uint8Array)
    expect(body).toContain('name="icon"; filename="icon.png"')
    expect(body).toContain('pixels')
    expect(body).not.toContain('/Users/me')
  })

  it('still hands a local entity’s daemon the path, exactly as before', async () => {
    render(<IconPopover {...baseProps} base="/v0/projects/p1" />)
    upload()

    await vi.waitFor(() =>
      expect(apiFetch).toHaveBeenCalledExactlyOnceWith('/v0/projects/p1/icon', {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: '/Users/me/icon.png' }),
      }),
    )
    expect(reads).toEqual([])
  })

  it('refuses an image over the 2 MB icon limit before reading or sending it', async () => {
    serveImage('x', 2 * 1024 * 1024 + 1)
    render(<IconPopover {...baseProps} base={REMOTE} />)
    upload()

    await vi.waitFor(() => expect(toast.error).toHaveBeenCalledOnce())
    expect(toast.error).toHaveBeenCalledWith(expect.stringMatching(/icon\.png.*2 MB/))
    expect(reads).toEqual(['HEAD asset://staged/Users/me/icon.png'])
    expect(apiFetch).not.toHaveBeenCalled()
  })

  // The popover may close while the OS dialog is up: the pick is still the user's.
  it('lands an image picked after the popover was closed', async () => {
    let pick!: (path: string) => void
    openDialog.mockReturnValue(new Promise((resolve) => (pick = resolve)))
    const { unmount } = render(<IconPopover {...baseProps} base={REMOTE} />)
    upload()

    unmount()
    pick('/Users/me/icon.png')

    await vi.waitFor(() => expect(apiFetch).toHaveBeenCalledOnce())
    expect(apiFetch.mock.calls[0]?.[0]).toBe(`${REMOTE}/icon`)
  })

  it('says the host is no longer paired, and sends it nothing, when it was removed meanwhile', async () => {
    const actual = await vi.importActual<typeof import('@/lib/api')>('@/lib/api')
    apiFetch.mockImplementation(actual.apiFetch)
    pairRemoteHost('h1', 'Studio Mac')
    let pick!: (path: string) => void
    openDialog.mockReturnValue(new Promise((resolve) => (pick = resolve)))
    render(<IconPopover {...baseProps} base={REMOTE} />)
    upload()

    useHostConfigStore.getState().removeHost('h1')
    pick('/Users/me/icon.png')

    await vi.waitFor(() =>
      expect(toast.error).toHaveBeenCalledExactlyOnceWith(
        'That host is no longer paired with this computer',
        'Pair it again in Settings → Hosts to use it.',
      ),
    )
    expect(sentTo('crowbar://')).toEqual([])
  })
})

describe('IconPopover — onStage mode stages locally instead of mutating the network', () => {
  it('emoji submit calls onStage, never apiFetch', () => {
    const onStage = vi.fn()
    render(<IconPopover {...baseProps} onStage={onStage} />)
    fireEvent.click(screen.getByRole('button', { name: 'Emoji' }))
    fireEvent.change(screen.getByPlaceholderText('Type an emoji…'), {
      target: { value: '🎉' },
    })
    fireEvent.click(screen.getByRole('button', { name: 'Set' }))

    expect(onStage).toHaveBeenCalledExactlyOnceWith({ kind: 'emoji', emoji: '🎉' })
    expect(apiFetch).not.toHaveBeenCalled()
  })

  it('reset calls onStage with kind reset, never apiFetch', () => {
    const onStage = vi.fn()
    render(<IconPopover {...baseProps} onStage={onStage} emoji="🎉" />)
    fireEvent.click(screen.getByRole('button', { name: 'Reset to default' }))

    expect(onStage).toHaveBeenCalledExactlyOnceWith({ kind: 'reset' })
    expect(apiFetch).not.toHaveBeenCalled()
  })

  // Desktop path: isTauri() true means Upload opens the native folder/file
  // dialog rather than the hidden <input>, same branch the real mutation
  // path already takes.
  it('a Tauri-picked upload path calls onStage with a convertFileSrc preview, never apiFetch', async () => {
    isTauri.mockReturnValue(true)
    openDialog.mockResolvedValue('/Users/me/icon.png')
    const onStage = vi.fn()
    render(<IconPopover {...baseProps} onStage={onStage} />)
    fireEvent.click(screen.getByRole('button', { name: 'Upload' }))

    await vi.waitFor(() =>
      expect(onStage).toHaveBeenCalledExactlyOnceWith({
        kind: 'path',
        path: '/Users/me/icon.png',
        previewUrl: 'asset://staged/Users/me/icon.png',
      }),
    )
    expect(apiFetch).not.toHaveBeenCalled()
  })

  // A staged preview URL (blob:/asset://) is a one-shot local pick, not the
  // app's own stable proxy URL — appending the usual cache-bust query param
  // to it would break it outright, so `src` must not do that in this mode.
  it('does not append a cache-busting query param to a staged iconUrl', () => {
    const onStage = vi.fn()
    render(
      <IconPopover {...baseProps} onStage={onStage} iconUrl="asset://staged/Users/me/icon.png" />,
    )
    // The mocked Popover renders trigger and content at once (both carry
    // their own <img>), so assert over all of them rather than assume one.
    const imgs = Array.from(document.querySelectorAll('img')) as HTMLImageElement[]
    expect(imgs.length).toBeGreaterThan(0)
    for (const img of imgs) {
      expect(img.src).not.toContain('?v=')
    }
    expect(imgs.some((img) => img.src === 'asset://staged/Users/me/icon.png')).toBe(true)
  })

  // Regression: the big preview used to render ONLY AvatarImage when `src`
  // was set, with no AvatarFallback sibling — Base UI's Fallback needs to
  // be mounted alongside Image to catch a failed load at all, so any `src`
  // that fails to load (the asset protocol was disabled when this first
  // surfaced; even enabled, a bad path still can) rendered a blank square
  // instead of falling back to the default glyph. Both must always be
  // mounted together when there's a `src`.
  it('mounts AvatarFallback alongside AvatarImage whenever src is set, so a failed load has somewhere to fall back to', () => {
    const onStage = vi.fn()
    render(
      <IconPopover
        {...baseProps}
        onStage={onStage}
        iconUrl="asset://staged/Users/me/icon.png"
        fallbackLarge={<span data-testid="fallback-large">fallback-large</span>}
      />,
    )
    expect(document.querySelector('img[src="asset://staged/Users/me/icon.png"]')).not.toBeNull()
    expect(screen.getByTestId('fallback-large')).toBeInTheDocument()
  })
})
