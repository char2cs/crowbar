import { StrictMode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { ContextMenu } from '@/components/ui/context-menu'

type TauriWindow = Window & { __TAURI_INTERNALS__?: { invoke: () => Promise<void> } }

afterEach(() => {
  cleanup()
  delete (window as TauriWindow).__TAURI_INTERNALS__
  vi.restoreAllMocks()
})

// The phone app runs in Tauri but has no OS context menu: the rendered one is the menu.
describe('ContextMenu — in the phone app', () => {
  it('renders the menu itself and pops no native one', async () => {
    ;(window as TauriWindow).__TAURI_INTERNALS__ = { invoke: vi.fn() }
    const fetchMock = vi.spyOn(globalThis, 'fetch')
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
    expect(fetchMock).not.toHaveBeenCalled()
  })
})

describe('ContextMenu — in a page the shell hosts', () => {
  const win = window as unknown as Record<string, unknown>

  beforeEach(() => {
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

  it('logs and falls back to the rendered, dismissible popup when the shell cannot pop one', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValue(new Error('shell gone'))
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

    await waitFor(() => expect(getByText('A')).toBeInTheDocument())
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      'Failed to show native context menu:',
      expect.any(Error),
    )
    // Closing on the failure itself would unmount the fallback before it rendered.
    expect(onClose).not.toHaveBeenCalled()
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })
})
