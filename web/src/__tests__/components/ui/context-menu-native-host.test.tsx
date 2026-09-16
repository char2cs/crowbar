import { StrictMode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { ContextMenu } from '@/components/ui/context-menu'

const MENU_RID = 7

const closeMock = vi.fn().mockResolvedValue(undefined)
const menuNewMock = vi.fn().mockResolvedValue({ rid: MENU_RID, close: closeMock })

vi.mock('@tauri-apps/api/menu', () => ({
  Menu: { new: (...args: unknown[]) => menuNewMock(...args) },
}))

// showNativeContextMenu (crowbar-bridge.ts) pops the menu through this app's
// own `popup_native_context_menu` command via window.__TAURI_INTERNALS__.invoke
// — not the JS `menu.popup()` method — because Tauri's built-in
// `plugin:menu|popup` command holds the webview's resources-table lock across
// the whole blocking popup call, deadlocking every other resource-backed
// command for as long as the menu stays open.
const invokeMock = vi.fn().mockResolvedValue(undefined)

type TauriWindow = Window & { __TAURI_INTERNALS__?: { invoke: typeof invokeMock } }

beforeEach(() => {
  closeMock.mockClear()
  menuNewMock.mockClear()
  menuNewMock.mockResolvedValue({ rid: MENU_RID, close: closeMock })
  invokeMock.mockClear()
  invokeMock.mockResolvedValue(undefined)
  ;(window as TauriWindow).__TAURI_INTERNALS__ = { invoke: invokeMock }
})

afterEach(() => {
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
      { items: Array<{ action: (id: string) => void }> },
    ]
    expect(nativeItems[0].action).toBeInstanceOf(Function)
    nativeItems[0].action('a')
    expect(onClick).toHaveBeenCalledOnce()

    await waitFor(() =>
      expect(invokeMock).toHaveBeenCalledExactlyOnceWith('popup_native_context_menu', {
        rid: MENU_RID,
        x: 42,
        y: 7,
      }),
    )
    await waitFor(() => expect(closeMock).toHaveBeenCalledOnce())
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

    await waitFor(() => expect(closeMock).toHaveBeenCalledOnce())
    expect(consoleErrorSpy).toHaveBeenCalledWith(
      'Failed to show native context menu:',
      expect.any(Error),
    )

    // The user must not be left with silence: a broken native popup call
    // falls back to the same rendered menu the non-Tauri path already uses.
    await waitFor(() => expect(getByText('A')).toBeInTheDocument())

    // onClose must NOT have fired on the failure path itself — doing so
    // would flip the caller's `isOpen` to false and unmount the fallback
    // before it ever got to render (every real call site gates its own
    // render on that same `isOpen`).
    expect(onClose).not.toHaveBeenCalled()

    // The fallback must be a real, usable menu: its own dismissal (Escape)
    // still calls the real onClose, exactly like the non-Tauri path.
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledOnce()
  })

  // Regression: a real right-click under React.StrictMode (this app wraps its
  // whole tree in it — src/main.tsx) double-invokes this effect synchronously
  // (setup → cleanup → setup again) before Menu.new()'s promise can resolve.
  // Without the cancellation check, BOTH invocations popped up a real native
  // menu — two live, stacked NSMenu tracking sessions from one right-click —
  // which read as "the menu reopens" after dismissing the top one.
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
    // StrictMode's double-invoke DOES call Menu.new() twice — that's expected
    // and harmless (the cancelled invocation's menu resource just gets closed
    // unopened, asserted below). What must stay singular is the actual popup.
    await waitFor(() => expect(menuNewMock).toHaveBeenCalledTimes(2))
    // Give any extra (buggy) popup call a chance to fire before asserting it didn't.
    await new Promise((resolve) => setTimeout(resolve, 20))
    expect(invokeMock).toHaveBeenCalledOnce()
    expect(closeMock).toHaveBeenCalledTimes(2)
  })
})
