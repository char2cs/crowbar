import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { ContextMenuItem } from '@/components/ui/context-menu'

const popupMock = vi.fn().mockResolvedValue(undefined)
const closeMock = vi.fn().mockResolvedValue(undefined)
const menuNewMock = vi.fn().mockResolvedValue({ popup: popupMock, close: closeMock })

vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: (...args: unknown[]) => menuNewMock(...args) },
}))

describe('showNativeContextMenu', () => {
  beforeEach(() => {
    popupMock.mockClear()
    closeMock.mockClear()
    menuNewMock.mockClear()
    menuNewMock.mockResolvedValue({ popup: popupMock, close: closeMock })
  })

  afterEach(() => {
    vi.resetModules()
  })

  it('maps a flat item list to MenuItemOptions and pops up at the given position', async () => {
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')
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
      { id: 'rename', text: 'Rename', enabled: true, accelerator: 'CmdOrCtrl+R', action: expect.any(Function) },
      { item: 'Separator' },
      { id: 'delete', text: 'Delete', enabled: false, accelerator: undefined, action: expect.any(Function) },
    ])

    const [renameEntry] = nativeItems as Array<{ action: (id: string) => void }>
    renameEntry.action('rename')
    expect(onClick).toHaveBeenCalledOnce()

    expect(popupMock).toHaveBeenCalledOnce()
    const [positionArg] = popupMock.mock.calls[0]
    expect(positionArg).toMatchObject({ x: 120, y: 80 })
  })

  it('maps nested items to a Submenu entry', async () => {
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')
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
          { id: 'turn-into-h1', text: 'Heading 1', enabled: true, accelerator: undefined, action: expect.any(Function) },
        ],
      },
    ])
  })

  it('closes the menu after popup resolves', async () => {
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')

    await showNativeContextMenu([{ id: 'a', label: 'A', onClick: vi.fn() }], { x: 0, y: 0 })

    expect(closeMock).toHaveBeenCalledOnce()
  })

  it('still closes the menu when popup rejects, and rethrows', async () => {
    popupMock.mockRejectedValueOnce(new Error('popup failed'))
    const { showNativeContextMenu } = await import('@/components/ui/context-menu')

    await expect(
      showNativeContextMenu([{ id: 'a', label: 'A', onClick: vi.fn() }], { x: 0, y: 0 }),
    ).rejects.toThrow('popup failed')
    expect(closeMock).toHaveBeenCalledOnce()
  })
})
