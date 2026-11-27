import { StrictMode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { act, cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { ContextMenu } from '@/components/ui/context-menu'

const MENU_RID = 7

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

const closeMock = vi.fn().mockResolvedValue(undefined)
const menuNewMock = vi.fn()

vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: (...args: unknown[]) => menuNewMock(...args) },
  MenuItem: {
    new: async (options: Record<string, unknown>) => ({ options, close: closeMock }),
  },
  Submenu: { new: async () => ({ close: closeMock }) },
  PredefinedMenuItem: { new: async () => ({ close: closeMock }) },
}))

const invokeMock = vi.fn().mockResolvedValue(undefined)

type TauriWindow = Window & { __TAURI_INTERNALS__?: { invoke: typeof invokeMock } }

beforeEach(() => {
  closeMock.mockClear()
  menuNewMock.mockReset()
  menuNewMock.mockResolvedValue({ rid: MENU_RID, close: closeMock })
  invokeMock.mockClear()
  invokeMock.mockResolvedValue(undefined)
  ;(window as TauriWindow).__TAURI_INTERNALS__ = { invoke: invokeMock }
})

afterEach(() => {
  platform.mobile = false
  cleanup()
  delete (window as TauriWindow).__TAURI_INTERNALS__
  vi.restoreAllMocks()
})

describe('ContextMenu — native path (isTauri() true)', () => {
  it('renders nothing and pops the native menu via popup_native_context_menu', async () => {
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
    const [{ items: nativeItems }] = menuNewMock.mock.calls[0] as [
      { items: Array<{ options: { action: () => void } }> },
    ]
    nativeItems[0].options.action()
    expect(onClick).toHaveBeenCalledOnce()

    await waitFor(() =>
      expect(invokeMock).toHaveBeenCalledExactlyOnceWith('popup_native_context_menu', {
        rid: MENU_RID,
        x: 42,
        y: 7,
      }),
    )
    await waitFor(() => expect(closeMock).toHaveBeenCalledTimes(2))
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

    await act(async () => {})
    expect(menuNewMock).not.toHaveBeenCalled()
    expect(invokeMock).not.toHaveBeenCalled()
    expect(onClose).not.toHaveBeenCalled()
  })

  it('logs and falls back to the rendered, dismissible popup when popup_native_context_menu rejects', async () => {
    invokeMock.mockRejectedValueOnce(new Error('popup failed'))
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    const onClose = vi.fn()

    const { getByText } = render(
      <ContextMenu
        isOpen
        position={{ x: 0, y: 0 }}
        items={[{ id: 'a', label: 'A', onClick: vi.fn() }]}
        onClose={onClose}
      />,
    )

    await waitFor(() => expect(closeMock).toHaveBeenCalledTimes(2))
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      'Failed to show native context menu:',
      expect.any(Error),
    )

    await waitFor(() => expect(getByText('A')).toBeInTheDocument())

    expect(onClose).not.toHaveBeenCalled()

    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })

  it('under StrictMode, only the surviving effect invocation pops up a menu', async () => {
    const onClose = vi.fn()
    render(
      <StrictMode>
        <ContextMenu
          isOpen
          position={{ x: 10, y: 20 }}
          items={[{ id: 'a', label: 'A', onClick: vi.fn() }]}
          onClose={onClose}
        />
      </StrictMode>,
    )

    await waitFor(() => expect(invokeMock).toHaveBeenCalledOnce())
    await waitFor(() => expect(menuNewMock).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(closeMock).toHaveBeenCalledTimes(2))
    expect(invokeMock).toHaveBeenCalledOnce()
  })
})

describe('ContextMenu — in the phone app', () => {
  it('renders the menu itself and pops no native one', async () => {
    platform.mobile = true
    const onClick = vi.fn()
    const { getByText } = render(
      <ContextMenu
        isOpen
        position={{ x: 4, y: 4 }}
        items={[{ id: 'a', label: 'Rename', onClick }]}
        onClose={vi.fn()}
      />,
    )

    fireEvent.click(getByText('Rename'))
    expect(onClick).toHaveBeenCalledOnce()
    expect(menuNewMock).not.toHaveBeenCalled()
    expect(invokeMock).not.toHaveBeenCalled()
  })
})

describe('ContextMenu — in a page the shell hosts', () => {
  const win = window as unknown as Record<string, unknown>

  beforeEach(() => {
    delete (window as TauriWindow).__TAURI_INTERNALS__
    win.crowbar = { post: vi.fn(), onmessage: null }
    win.__CROWBAR__ = { api: 'crowbar://local', host: 'local' }
  })

  afterEach(() => {
    delete win.crowbar
    delete win.__CROWBAR__
  })

  it('asks the shell for its native menu and runs the entry it answers', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValue(new Response(JSON.stringify({ path: [2, 1] })))
    const onClose = vi.fn()
    const center = vi.fn()
    const { container } = render(
      <StrictMode>
        <ContextMenu
          isOpen
          position={{ x: 12, y: 34 }}
          items={[
            { id: 'delete', label: 'Delete', onClick: vi.fn() },
            { id: 'sep', label: '', separator: true, onClick: vi.fn() },
            {
              id: 'align',
              label: 'Align',
              onClick: vi.fn(),
              items: [
                { id: 'left', label: 'Left', onClick: vi.fn(), disabled: true },
                { id: 'center', label: 'Center', onClick: center },
              ],
            },
          ]}
          onClose={onClose}
        />
      </StrictMode>,
    )

    expect(container).toBeEmptyDOMElement()
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
    expect(center).toHaveBeenCalledOnce()
    expect(fetchMock).toHaveBeenCalledOnce()
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit]
    expect(url).toBe('crowbar://local/__crowbar/menu')
    expect(JSON.parse(init.body as string)).toEqual({
      x: 12,
      y: 34,
      entries: [
        { kind: 'item', label: 'Delete', enabled: true },
        { kind: 'separator' },
        {
          kind: 'submenu',
          label: 'Align',
          enabled: true,
          entries: [
            { kind: 'item', label: 'Left', enabled: false },
            { kind: 'item', label: 'Center', enabled: true },
          ],
        },
      ],
    })
  })

  it('runs nothing when the menu is dismissed', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValue(new Response(JSON.stringify({ path: null })))
    const onClose = vi.fn()
    const onClick = vi.fn()
    render(
      <ContextMenu
        isOpen
        position={{ x: 1, y: 2 }}
        items={[{ id: 'a', label: 'A', onClick }]}
        onClose={onClose}
      />,
    )
    await waitFor(() => expect(onClose).toHaveBeenCalledOnce())
    expect(onClick).not.toHaveBeenCalled()
  })
})
