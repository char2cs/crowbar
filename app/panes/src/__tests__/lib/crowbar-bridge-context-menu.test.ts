import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ContextMenuItem } from '@/components/ui/context-menu'

const MENU_RID = 42

interface FakeResource {
  kind: string
  rid: number
  options: Record<string, unknown>
  close: () => Promise<void>
}

let nextRid = 100
const closeMock = vi.fn().mockResolvedValue(undefined)
const fakeResource = (kind: string, options: Record<string, unknown>): FakeResource => ({
  kind,
  rid: kind === 'Menu' ? MENU_RID : nextRid++,
  options,
  close: closeMock,
})

// Tauri 2.12 drops an item's click channel as soon as the item's Rust wrapper
// is dropped, and an item passed INLINE to `Menu.new` is dropped right after
// the menu is built. Only items created through their own constructor stay in
// the resource table and keep a live click channel, so the bridge must build
// every entry that way and hand `Menu.new` the instances.
vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: async (o: Record<string, unknown>) => fakeResource('Menu', o) },
  MenuItem: { new: async (o: Record<string, unknown>) => fakeResource('MenuItem', o) },
  Submenu: { new: async (o: Record<string, unknown>) => fakeResource('Submenu', o) },
  PredefinedMenuItem: {
    new: async (o: Record<string, unknown>) => fakeResource('PredefinedMenuItem', o),
  },
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
    nextRid = 100
    invokeMock.mockResolvedValue(undefined)
    ;(window as TauriWindow).__TAURI_INTERNALS__ = { invoke: invokeMock }
  })

  afterEach(() => {
    delete (window as TauriWindow).__TAURI_INTERNALS__
    vi.restoreAllMocks()
    vi.resetModules()
  })

  async function show(items: ContextMenuItem[], isCancelled?: () => boolean) {
    const { Menu } = await import('@tauri-apps/api/menu')
    const menuNew = vi.spyOn(Menu, 'new')
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')
    await showNativeContextMenu(items, { x: 120, y: 80 }, isCancelled)
    const built = menuNew.mock.results.at(-1)
    const menu = built ? ((await built.value) as FakeResource) : null
    return { menu, entries: (menu?.options.items ?? []) as FakeResource[] }
  }

  it('builds each entry through its own constructor and pops up via popup_native_context_menu', async () => {
    const onClick = vi.fn()
    const { entries } = await show([
      { id: 'rename', label: 'Rename', onClick, shortcut: 'CmdOrCtrl+R' },
      { id: 'sep', label: '', separator: true, onClick: () => {} },
      { id: 'delete', label: 'Delete', onClick: vi.fn(), disabled: true },
    ])

    expect(entries.map((e) => e.kind)).toEqual(['MenuItem', 'PredefinedMenuItem', 'MenuItem'])
    expect(entries[0].options).toMatchObject({
      id: expect.stringMatching(/^\d+:rename$/),
      text: 'Rename',
      enabled: true,
      accelerator: 'CmdOrCtrl+R',
    })
    expect(entries[1].options).toEqual({ item: 'Separator' })
    expect(entries[2].options).toMatchObject({
      id: expect.stringMatching(/:delete$/),
      text: 'Delete',
      enabled: false,
    })

    ;(entries[0].options.action as () => void)()
    expect(onClick).toHaveBeenCalledOnce()

    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('popup_native_context_menu', {
      rid: MENU_RID,
      x: 120,
      y: 80,
    })
  })

  it('builds nested items as a Submenu whose children are item instances', async () => {
    const onClick = vi.fn()
    const { entries } = await show([
      {
        id: 'turn-into',
        label: 'Turn into',
        onClick: () => {},
        items: [{ id: 'turn-into-h1', label: 'Heading 1', onClick }],
      },
    ])

    expect(entries.map((e) => e.kind)).toEqual(['Submenu'])
    expect(entries[0].options).toMatchObject({ text: 'Turn into', enabled: true })
    const children = entries[0].options.items as FakeResource[]
    expect(children.map((c) => c.kind)).toEqual(['MenuItem'])
    expect(children[0].options).toMatchObject({
      id: expect.stringMatching(/:turn-into-h1$/),
      text: 'Heading 1',
    })
    ;(children[0].options.action as () => void)()
    expect(onClick).toHaveBeenCalledOnce()
  })

  // Tauri delivers a click to the item's action via the event loop, after the
  // blocking popup call has already returned, and closing an item removes its
  // click channel. So nothing is closed on popup return.
  it('keeps every resource alive after popup resolves and closes them once the chosen action has run', async () => {
    const onClick = vi.fn()
    const { entries } = await show([
      { id: 'a', label: 'A', onClick },
      { id: 'b', label: 'B', onClick: vi.fn() },
    ])

    expect(closeMock).not.toHaveBeenCalled()
    ;(entries[0].options.action as () => void)()
    expect(onClick).toHaveBeenCalledOnce()
    // menu + two items
    await vi.waitFor(() => expect(closeMock).toHaveBeenCalledTimes(3))
  })

  it('closes a dismissed menu and its items when the next one is shown', async () => {
    const item: ContextMenuItem = { id: 'a', label: 'A', onClick: vi.fn() }
    const { showNativeContextMenu } = await import('@/lib/crowbar-bridge')

    await showNativeContextMenu([item], { x: 0, y: 0 })
    expect(closeMock).not.toHaveBeenCalled()

    await showNativeContextMenu([item], { x: 0, y: 0 })
    expect(closeMock).toHaveBeenCalledTimes(2)
  })

  it('closes everything when popup_native_context_menu rejects, and rethrows', async () => {
    invokeMock.mockRejectedValueOnce(new Error('popup failed'))

    await expect(show([{ id: 'a', label: 'A', onClick: vi.fn() }])).rejects.toThrow('popup failed')
    expect(closeMock).toHaveBeenCalledTimes(2)
  })

  // React StrictMode double-invokes effects (setup → cleanup → setup again)
  // synchronously, before this function's first `await` can resolve. Without
  // the `isCancelled` check, BOTH invocations would go on to build a menu and
  // invoke a REAL popup — two live, stacked native menus from one right-click.
  it('closes everything without popping it up when cancelled before the menu is built', async () => {
    await show([{ id: 'a', label: 'A', onClick: vi.fn() }], () => true)

    expect(invokeMock).not.toHaveBeenCalled()
    expect(closeMock).toHaveBeenCalledTimes(2)
  })

  // Tauri keys an item's click channel by the item's id, and dropping any item
  // removes the channel registered under that id. Two menus built from the same
  // rows (React StrictMode double-invokes the open effect) must not share ids,
  // or closing the cancelled one silences the one that is popped up.
  it('gives every menu its own item ids, even when built from the same rows', async () => {
    const item: ContextMenuItem = { id: 'rename', label: 'Rename', onClick: vi.fn() }
    const first = await show([item])
    const second = await show([item])

    const ids = [first, second].map((m) => m.entries[0].options.id)
    expect(ids[0]).not.toEqual(ids[1])
  })

  it('pops up normally when isCancelled reports false', async () => {
    await show([{ id: 'a', label: 'A', onClick: vi.fn() }], () => false)

    expect(invokeMock).toHaveBeenCalledExactlyOnceWith('popup_native_context_menu', {
      rid: MENU_RID,
      x: 120,
      y: 80,
    })
    expect(closeMock).not.toHaveBeenCalled()
  })
})
