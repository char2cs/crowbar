import type React from 'react'
import { render, screen, fireEvent } from '@testing-library/react'
import { describe, it, expect, vi, beforeEach } from 'vitest'

// @base-ui/react ships pure ESM and gets its own React copy in the
// vitest/jsdom process (same trap as Tabs — see sidebar-carousel.test.tsx's
// own doc on this). Popover/Avatar are mocked to plain, always-"open"
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
vi.mock('@/lib/native-dialog', () => ({ openNativeDialog: (...args: unknown[]) => openDialog(...args) }))

const isTauri = vi.fn(() => false)
vi.mock('@/lib/crowbar-bridge', () => ({ isTauri: () => isTauri() }))

vi.mock('@tauri-apps/api/core', () => ({
  convertFileSrc: (path: string) => `asset://staged${path}`,
}))

vi.mock('@/features/window/stores/toast-store', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

import { IconPopover } from '@/components/layout/icon-popover'

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
      <IconPopover
        {...baseProps}
        onStage={onStage}
        iconUrl="asset://staged/Users/me/icon.png"
      />,
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
