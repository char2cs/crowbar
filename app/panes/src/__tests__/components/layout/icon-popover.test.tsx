import type React from 'react'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'

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

import * as fetchApi from '@/lib/api/fetch'
import * as nativeDialog from '@/lib/native-dialog'
import * as bridge from '@/lib/crowbar-bridge'

const realApiFetch = fetchApi.apiFetch
const apiFetch = vi.fn().mockResolvedValue(undefined)
vi.spyOn(fetchApi, 'apiFetch').mockImplementation((...args) => apiFetch(...args))
const openDialog = vi.fn()
vi.spyOn(nativeDialog, 'openNativeDialog').mockImplementation((...args) => openDialog(...args))
const isTauri = vi.fn(() => false)
vi.spyOn(bridge, 'isTauri').mockImplementation(() => isTauri())
vi.spyOn(bridge, 'convertFileSrc').mockImplementation((path) => `asset://staged${path}`)

import { IconPopover } from '@/components/layout/icon-popover'
import {
  expectMigratingShown,
  expectReported,
  migratingRefusal,
  refusal,
  watchToasts,
} from '@/__tests__/__fixtures__/reports'
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

describe('IconPopover — editing an icon', () => {
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

  it('a refused reset reports the daemon’s reason', async () => {
    const toasts = watchToasts()
    apiFetch.mockRejectedValueOnce(refusal('icon is read-only'))
    render(<IconPopover {...baseProps} base="/v0/projects/p1" emoji="🚀" />)
    fireEvent.click(screen.getByRole('button', { name: 'Reset to default' }))

    await waitFor(() => expectReported(toasts, 'Failed to reset icon', 'icon is read-only'))
  })

  it('a reset refused while the daemon migrates shows the migrating state, not a toast', async () => {
    const toasts = watchToasts()
    apiFetch.mockRejectedValueOnce(migratingRefusal())
    render(<IconPopover {...baseProps} base="/v0/projects/p1" emoji="🚀" />)
    fireEvent.click(screen.getByRole('button', { name: 'Reset to default' }))

    await waitFor(() => expectMigratingShown(toasts))
  })

  it('the trigger opts out of inline baseline alignment for bare-span callers', () => {
    render(<IconPopover {...baseProps} base="/v0/projects/p1" />)
    expect(screen.getByRole('button', { name: 'Edit my-project icon' }).className).toContain(
      'align-middle',
    )
  })
})

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

describe('IconPopover — an upload to a remote entity sends the image, never its path', () => {
  const REMOTE = 'crowbar://h1/v0/projects/p1'
  const reads: string[] = []
  const sentTo = (prefix: string) =>
    vi.mocked(fetch).mock.calls.filter(([url]) => String(url).startsWith(prefix))

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
    const toasts = watchToasts()
    serveImage('x', 2 * 1024 * 1024 + 1)
    render(<IconPopover {...baseProps} base={REMOTE} />)
    upload()

    await waitFor(() =>
      expectReported(toasts, 'Failed to set icon', 'icon.png is larger than the 2 MB icon limit'),
    )
    expect(reads).toEqual(['HEAD asset://staged/Users/me/icon.png'])
    expect(apiFetch).not.toHaveBeenCalled()
  })

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
    const toasts = watchToasts()
    apiFetch.mockImplementation(realApiFetch)
    pairRemoteHost('h1', 'Studio Mac')
    let pick!: (path: string) => void
    openDialog.mockReturnValue(new Promise((resolve) => (pick = resolve)))
    render(<IconPopover {...baseProps} base={REMOTE} />)
    upload()

    useHostConfigStore.getState().removeHost('h1')
    pick('/Users/me/icon.png')

    await vi.waitFor(() =>
      expect(toasts).toHaveBeenCalledWith(
        expect.objectContaining({
          type: 'error',
          description: expect.stringMatching(/no longer paired/),
        }),
      ),
    )
    expect(sentTo('crowbar://')).toEqual([])
  })
})

describe('IconPopover — a stored icon', () => {
  it('mounts AvatarFallback alongside AvatarImage, so a failed load has somewhere to fall back to', () => {
    render(
      <IconPopover
        {...baseProps}
        base="/v0/projects/p1"
        iconUrl="/v0/projects/p1/icon"
        fallbackLarge={<span data-testid="fallback-large">fallback-large</span>}
      />,
    )
    expect(document.querySelector('img[src^="/v0/projects/p1/icon"]')).not.toBeNull()
    expect(screen.getByTestId('fallback-large')).toBeInTheDocument()
  })
})
