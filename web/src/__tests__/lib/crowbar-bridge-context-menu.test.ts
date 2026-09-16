import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ContextMenuItem } from '@/components/ui/context-menu'

const MENU_RID = 42

const closeMock = vi.fn().mockResolvedValue(undefined)
const menuNewMock = vi.fn().mockResolvedValue({ rid: MENU_RID, close: closeMock })

vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: (...args: unknown[]) => menuNewMock(...args) },
}))

// showNativeContextMenu pops the menu via `popup_native_context_menu`, this
// app's own command — not the JS `menu.popup()` method — because Tauri's
// built-in `plugin:menu|popup` command holds the webview's resources-table
// lock across the whole blocking popup call, deadlocking every other
// resource-backed command for as long as the menu stays open. See the doc
// comment on `showNativeContextMenu` and on `popup_native_context_menu` in
// `desktop/src-tauri/src/lib.rs`.
const invokeMock = vi.fn().mockResolvedValue(undefined)

type TauriWindow = Window & { __TAURI_INTERNALS__?: { invoke: typeof invokeMock } }

describe('showNativeContextMenu', () => {
  beforeEach(() => {
    invokeMock.mockClear()
    closeMock.mockClear()
    menuNewMock.mockClear()
    menuNewMock.mockResolvedValue({ rid: MENU_RID, close: closeMock })
    invokeMock.mockResolvedValue(undefined)
    ;(window as TauriWindow).__TAURI_INTERNALS__ = { invoke: invokeMock }
  })

  afterEach(() => {
    delete (window as TauriWindow).__TAURI_INTERNALS__
    vi.resetModules()
  })

  it('maps a flat item list to MenuItemOptions and pops up via popup_native_context_menu', async () => {
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')
    const onClick = vi.fn()
    const items: ContextMenuItem[] = [
      { id: 'rename', label: 'Rename', onClick, shortcut: 'CmdOrCtrl+R' },
      { id: 'sep', label: '', separator: true, onClick: () => {} },
      { id: 'delete', label: 'Delete', onClick: vi.fn(), disabled: true },
    ]

    await showNativeContextMenu(items, { x: 120, y: 80 })

    expect(menuNewMock).toHaveBeenCalledTimes(1)
    const [{ items: nativeItems }] = menuNewMock.mock.calls[0] as [{ items: unknown[] }]
    expect(nativeItems).toEqual([
      {
        id: 'rename',
        text: 'Rename',
        enabled: true,
        accelerator: 'CmdOrCtrl+R',
        action: expect.any(Function),
      },
      { item: 'Separator' },
      {
        id: 'delete',
        text: 'Delete',
        enabled: false,
        accelerator: undefined,
        action: expect.any(Function),
      },
    ])

    const [renameEntry] = nativeItems as Array<{ action: (id: string) => void }>
    renameEntry.action('rename')
    expect(onClick).toHaveBeenCalledOnce()

    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('popup_native_context_menu', {
      rid: MENU_RID,
      x: 120,
      y: 80,
    })
  })

  it('maps nested items to a Submenu entry', async () => {
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')
    const items: ContextMenuItem[] = [
      {
        id: 'turn-into',
        label: 'Turn into',
        onClick: () => {},
        items: [{ id: 'turn-into-h1', label: 'Heading 1', onClick: vi.fn() }],
      },
    ]

    await showNativeContextMenu(items, { x: 0, y: 0 })

    const [{ items: nativeItems }] = menuNewMock.mock.calls[0] as [{ items: unknown[] }]
    expect(nativeItems).toEqual([
      {
        text: 'Turn into',
        enabled: true,
        items: [
          {
            id: 'turn-into-h1',
            text: 'Heading 1',
            enabled: true,
            accelerator: undefined,
            action: expect.any(Function),
          },
        ],
      },
    ])
  })

  it('closes the menu after popup resolves', async () => {
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')

    await showNativeContextMenu([{ id: 'a', label: 'A', onClick: vi.fn() }], { x: 0, y: 0 })

    expect(closeMock).toHaveBeenCalledOnce()
  })

  it('still closes the menu when popup_native_context_menu rejects, and rethrows', async () => {
    invokeMock.mockRejectedValueOnce(new Error('popup failed'))
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')

    await expect(
      showNativeContextMenu([{ id: 'a', label: 'A', onClick: vi.fn() }], { x: 0, y: 0 }),
    ).rejects.toThrow('popup failed')
    expect(closeMock).toHaveBeenCalledOnce()
  })

  // React StrictMode double-invokes effects (setup → cleanup → setup again)
  // synchronously, before this function's first `await` can resolve. Without
  // the `isCancelled` check, BOTH invocations would go on to build a menu and
  // invoke a REAL popup — two live, stacked native menus from one right-click.
  it('closes the menu without popping it up when cancelled before Menu.new() resolves', async () => {
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')

    await showNativeContextMenu(
      [{ id: 'a', label: 'A', onClick: vi.fn() }],
      { x: 0, y: 0 },
      () => true,
    )

    expect(invokeMock).not.toHaveBeenCalled()
    expect(closeMock).toHaveBeenCalledOnce()
  })

  it('pops up normally when isCancelled reports false', async () => {
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')

    await showNativeContextMenu(
      [{ id: 'a', label: 'A', onClick: vi.fn() }],
      { x: 0, y: 0 },
      () => false,
    )

    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('popup_native_context_menu', {
      rid: MENU_RID,
      x: 0,
      y: 0,
    })
    expect(closeMock).toHaveBeenCalledOnce()
  })
})
