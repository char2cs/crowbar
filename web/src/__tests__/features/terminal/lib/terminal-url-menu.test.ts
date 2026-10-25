import { afterEach, describe, expect, it, vi } from 'vitest'
import type { ContextMenuItem } from '@/components/ui/context-menu'
import { windowPaneStore } from '@/features/panes/stores/window-pane-store'
import { showTerminalUrlMenu } from '@/features/terminal/lib/terminal-url-menu'

const shown = vi.hoisted(() => ({ items: [] as ContextMenuItem[], position: null as unknown }))
const openExternalUrl = vi.hoisted(() => vi.fn(() => Promise.resolve()))

vi.mock('@/lib/crowbar-bridge', () => ({
  showNativeContextMenu: vi.fn((items: ContextMenuItem[], position: unknown) => {
    shown.items = items
    shown.position = position
    return Promise.resolve()
  }),
}))
vi.mock('@/lib/external-open', () => ({ openExternalUrl }))

const url = 'https://a.test/x'
const at = { x: 12, y: 34 }

afterEach(() => vi.restoreAllMocks())

const item = (id: string) => {
  const found = shown.items.find((i) => i.id === id)
  if (!found) throw new Error(`no ${id} item`)
  return found
}

describe('showTerminalUrlMenu', () => {
  it('pops a native menu at the pointer with the three link actions', async () => {
    await showTerminalUrlMenu(url, at, { workspaceId: 'ws-1', paneId: 'pane-1' })

    expect(shown.position).toEqual(at)
    expect(shown.items.map((i) => i.label)).toEqual([
      'Open in Default Browser',
      'Open in Crowbar Browser',
      'Copy Link',
    ])
  })

  it('opens the default browser through the external opener', async () => {
    await showTerminalUrlMenu(url, at, null)
    item('open-default-browser').onClick()
    expect(openExternalUrl).toHaveBeenCalledExactlyOnceWith(url)
  })

  it('opens a browser tab in the terminal own pane and workspace', async () => {
    const openContent = vi.fn()
    vi.spyOn(windowPaneStore, 'getState').mockReturnValue({
      bufferActions: { openContent },
    } as unknown as ReturnType<typeof windowPaneStore.getState>)

    await showTerminalUrlMenu(url, at, { workspaceId: 'ws-1', paneId: 'pane-1' })
    item('open-crowbar-browser').onClick()

    expect(openContent).toHaveBeenCalledExactlyOnceWith(
      { type: 'browser', url, workspaceId: 'ws-1' },
      { paneId: 'pane-1' },
    )
  })

  it('disables the Crowbar browser item when the terminal has no known view', async () => {
    await showTerminalUrlMenu(url, at, null)
    expect(item('open-crowbar-browser').disabled).toBe(true)
  })

  it('copies the link', async () => {
    const writeText = vi.fn(() => Promise.resolve())
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })

    await showTerminalUrlMenu(url, at, null)
    item('copy-link').onClick()

    expect(writeText).toHaveBeenCalledExactlyOnceWith(url)
  })
})
