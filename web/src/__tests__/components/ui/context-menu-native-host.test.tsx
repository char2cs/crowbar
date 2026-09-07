import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, render, waitFor } from '@testing-library/react'
import { ContextMenu } from '@/components/ui/context-menu'

const popupMock = vi.fn().mockResolvedValue(undefined)
const closeMock = vi.fn().mockResolvedValue(undefined)
const menuNewMock = vi.fn().mockResolvedValue({ popup: popupMock, close: closeMock })

vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: (...args: unknown[]) => menuNewMock(...args) },
}))

type TauriWindow = Window & { __TAURI_INTERNALS__?: object }

beforeEach(() => {
  popupMock.mockClear()
  closeMock.mockClear()
  menuNewMock.mockClear()
  menuNewMock.mockResolvedValue({ popup: popupMock, close: closeMock })
  ;(window as TauriWindow).__TAURI_INTERNALS__ = {}
})

afterEach(() => {
  cleanup()
  delete (window as TauriWindow).__TAURI_INTERNALS__
  vi.restoreAllMocks()
})

describe('ContextMenu — native path (isTauri() true)', () => {
  it('renders nothing and pops the native menu with the right items and position', async () => {
    const onClose = vi.fn()
    const onClick = vi.fn()
    const { container } = render(
      <ContextMenu
        isOpen
        position={{ x: 42, y: 7 }}
        items={[{ id: 'a', label: 'A', onClick }]}
        onClose={onClose}
      />,
    )

    expect(container).toBeEmptyDOMElement()

    await waitFor(() => expect(menuNewMock).toHaveBeenCalledOnce())
    const [{ items: nativeItems }] = menuNewMock.mock.calls[0] as [{ items: Array<{ action: (id: string) => void }> }]
    expect(nativeItems[0].action).toBeInstanceOf(Function)
    nativeItems[0].action('a')
    expect(onClick).toHaveBeenCalledOnce()

    await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
  })

  it('does nothing when isOpen is false', async () => {
    const onClose = vi.fn()
    render(
      <ContextMenu
        isOpen={false}
        position={{ x: 0, y: 0 }}
        items={[{ id: 'a', label: 'A', onClick: vi.fn() }]}
        onClose={onClose}
      />,
    )

    await new Promise((resolve) => setTimeout(resolve, 0))
    expect(menuNewMock).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
  })
})
