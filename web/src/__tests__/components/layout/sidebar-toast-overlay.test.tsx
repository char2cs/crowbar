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
