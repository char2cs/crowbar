/**
 * The sidebar-OPEN branch windows the toast list. base-ui inserts newest-first,
 * so a plain `slice(0, 3)` keeps the three NEWEST and silently discards the rest.
 * ConnectionIndicator raises its "Backend unavailable" toast with `duration: 0`
 * (timeout 0 — removable only by a reconnect), so an outage that also produces
 * three failure toasts pushed the one toast that says WHY off the screen, with
 * nothing left to bring it back.
 */
import { act, cleanup, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { SidebarToastOverlay } from '@/components/layout/sidebar-toast-overlay'
import { toastManager } from '@/lib/toast-manager'
import { ApiError } from '@/lib/errors/api-error'
import { report } from '@/lib/errors/report'

const OUTAGE = 'Backend unavailable'

function addPinned(title: string): string {
  let id = ''
  act(() => {
    id = toastManager.add({ title, type: 'warning', timeout: 0 })
  })
  return id
}

function addTransient(title: string): string {
  let id = ''
  act(() => {
    id = toastManager.add({ title, type: 'error' })
  })
  return id
}

const opened: string[] = []

beforeEach(() => {
  opened.length = 0
})

afterEach(() => {
  act(() => {
    for (const id of opened) toastManager.close(id)
  })
  cleanup()
})

function track(id: string): string {
  opened.push(id)
  return id
}

describe('SidebarToastOverlay with the sidebar open', () => {
  it('keeps a pinned toast on screen when three transient ones follow it', () => {
    render(<SidebarToastOverlay sidebarOpen sidebarSide="left" />)

    track(addPinned(OUTAGE))
    track(addTransient('Failed to fetch'))
    track(addTransient('Failed to refresh workspaces'))
    track(addTransient('Conflict'))

    expect(screen.getByText(OUTAGE)).toBeInTheDocument()
  })

  it('still caps how many toasts are on screen at once', () => {
    render(<SidebarToastOverlay sidebarOpen sidebarSide="left" />)

    track(addPinned(OUTAGE))
    track(addTransient('one'))
    track(addTransient('two'))
    track(addTransient('three'))

    expect(document.querySelectorAll('[data-slot="toast-title"]')).toHaveLength(3)
  })

  it('spends the remaining slots on the NEWEST transient toasts', () => {
    render(<SidebarToastOverlay sidebarOpen sidebarSide="left" />)

    track(addPinned(OUTAGE))
    track(addTransient('oldest'))
    track(addTransient('middle'))
    track(addTransient('newest'))

    expect(screen.getByText(OUTAGE)).toBeInTheDocument()
    expect(screen.getByText('newest')).toBeInTheDocument()
    expect(screen.getByText('middle')).toBeInTheDocument()
    expect(screen.queryByText('oldest')).toBeNull()
  })

  it('shows three transient toasts when nothing is pinned', () => {
    render(<SidebarToastOverlay sidebarOpen sidebarSide="left" />)

    track(addTransient('one'))
    track(addTransient('two'))
    track(addTransient('three'))

    expect(document.querySelectorAll('[data-slot="toast-title"]')).toHaveLength(3)
  })
})

describe('SidebarToastOverlay mounting after a report', () => {
  it('shows a failure reported before any viewport was mounted', () => {
    report(new ApiError('state file is unreadable', 500), { action: 'Restore the last session' })
    opened.push('Restore the last session')

    render(<SidebarToastOverlay sidebarOpen sidebarSide="left" />)

    expect(screen.getByText('Restore the last session')).toBeInTheDocument()
    expect(screen.getByText('state file is unreadable')).toBeInTheDocument()
  })

  it('shows a promise toast started before any viewport was mounted, and its outcome', async () => {
    let finish!: () => void
    const saving = new Promise<void>((resolve) => (finish = resolve))
    void toastManager.promise(saving, { loading: 'Saving', success: 'Saved', error: 'Not saved' })

    render(<SidebarToastOverlay sidebarOpen sidebarSide="left" />)
    expect(screen.getByText('Saving')).toBeInTheDocument()
    await act(async () => finish())

    expect(await screen.findByText('Saved')).toBeInTheDocument()
    act(() => toastManager.close())
  })

  it('does not show a held toast that was closed before the mount', () => {
    act(() => toastManager.close(toastManager.add({ title: 'superseded', type: 'info' })))

    render(<SidebarToastOverlay sidebarOpen sidebarSide="left" />)

    expect(screen.queryByText('superseded')).toBeNull()
  })
})
