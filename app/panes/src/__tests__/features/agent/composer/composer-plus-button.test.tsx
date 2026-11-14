import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { ComposerPlusButton } from '@/features/agent/composer/composer-plus-button'

const platform = vi.hoisted(() => ({ mobile: false }))
vi.mock('@/utils/platform', async (importOriginal) => ({
  ...(await importOriginal<typeof import('@/utils/platform')>()),
  get IS_MOBILE() {
    return platform.mobile
  },
}))

const chrome = vi.hoisted(() => ({
  native: false,
  show: vi.fn(),
  listener: null as null | ((intent: { type: string; id?: string }) => void),
}))
vi.mock('@/lib/native-chrome', () => ({
  hasNativeChrome: () => chrome.native,
  setNativePlusMenu: chrome.show,
  onNativeChromeIntent: (cb: typeof chrome.listener) => {
    chrome.listener = cb
    return () => {
      chrome.listener = null
    }
  },
}))

afterEach(() => {
  cleanup()
  platform.mobile = false
  chrome.native = false
  chrome.show.mockClear()
})

function draw(overrides: Partial<Parameters<typeof ComposerPlusButton>[0]> = {}) {
  const props = {
    onOpenExcalidraw: vi.fn(),
    onOpenAttachFile: vi.fn(),
    onFiles: vi.fn(),
    onMentionFile: vi.fn(),
    onStartCommand: vi.fn(),
    ...overrides,
  }
  return { ...render(<ComposerPlusButton {...props} />), props }
}

const names = (items: HTMLElement[]) =>
  items.map((el) => el.querySelector('span')?.textContent ?? '')

describe('ComposerPlusButton on a desktop', () => {
  it('opens the board menu: attach, paste, drawing, then mention and command', async () => {
    const user = userEvent.setup()
    draw()
    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    expect(names(await screen.findAllByRole('menuitem'))).toEqual([
      'Attach files…',
      'Paste image',
      'New drawing',
      'Mention a file',
      'Run a command',
    ])
  })

  it.each([
    ['new drawing', 'onOpenExcalidraw', undefined],
    ['attach files', 'onOpenAttachFile', undefined],
    ['mention a file', 'onMentionFile', undefined],
    ['run a command', 'onStartCommand', undefined],
  ] as const)('routes "%s" to %s', async (row, action, arg) => {
    const user = userEvent.setup()
    const { props } = draw()
    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: new RegExp(row, 'i') }))
    expect(props[action]).toHaveBeenCalledTimes(1)
    if (arg) expect(props[action]).toHaveBeenCalledWith(arg)
  })

  it('hands the clipboard image to the composer as a file', async () => {
    const png = new Blob(['x'], { type: 'image/png' })
    const read = vi.fn(async () => [{ types: ['image/png'], getType: async () => png }])
    const user = userEvent.setup()
    Object.defineProperty(navigator, 'clipboard', { value: { read }, configurable: true })
    const { props } = draw()
    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /paste image/i }))
    await waitFor(() => expect(props.onFiles).toHaveBeenCalledTimes(1))
    const [files] = vi.mocked(props.onFiles).mock.calls[0]!
    expect(files.map((file: File) => file.name)).toEqual(['pasted-image.png'])
  })

  it('hands nothing over when the clipboard holds no image', async () => {
    const read = vi.fn(async () => [{ types: ['text/plain'], getType: async () => new Blob() }])
    const user = userEvent.setup()
    Object.defineProperty(navigator, 'clipboard', { value: { read }, configurable: true })
    const { props } = draw()
    await user.click(screen.getByRole('button', { name: /add to this message/i }))
    await user.click(await screen.findByRole('menuitem', { name: /paste image/i }))
    await waitFor(() => expect(read).toHaveBeenCalled())
    expect(props.onFiles).not.toHaveBeenCalled()
  })

  it('marks the trigger data-open only while the menu is open', async () => {
    const user = userEvent.setup()
    draw()
    const trigger = screen.getByRole('button', { name: /add to this message/i })
    expect(trigger).not.toHaveAttribute('data-open')
    await user.click(trigger)
    await screen.findByRole('menuitem', { name: /new drawing/i })
    expect(trigger).toHaveAttribute('data-open')
  })
})

describe('ComposerPlusButton on the phone', () => {
  const openSheet = async (overrides: Parameters<typeof draw>[0] = {}) => {
    platform.mobile = true
    chrome.native = true
    const view = draw(overrides)
    await userEvent.setup().click(screen.getByRole('button', { name: /add to this message/i }))
    return view
  }

  it('hands the native sheet the board rows: photo library, camera, attach, drawing, then the pickers', async () => {
    await openSheet()
    expect(screen.queryByRole('menuitem')).toBeNull()
    expect(chrome.show).toHaveBeenLastCalledWith({
      open: true,
      title: 'Add to this message',
      items: [
        { id: 'photos', title: 'Photo library', icon: 'Images', hint: null, divider: false },
        { id: 'camera', title: 'Take a photo', icon: 'Camera', hint: null, divider: false },
        { id: 'attach', title: 'Attach files', icon: 'Paperclip', hint: null, divider: false },
        {
          id: 'drawing',
          title: 'New drawing',
          icon: 'PencilSimpleLine',
          hint: null,
          divider: false,
        },
        { id: 'mention', title: 'Mention a file', icon: 'At', hint: '@', divider: true },
        {
          id: 'command',
          title: 'Run a command',
          icon: 'TerminalWindow',
          hint: '/',
          divider: false,
        },
      ],
    })
  })

  it.each([
    ['drawing', 'onOpenExcalidraw'],
    ['attach', 'onOpenAttachFile'],
    ['mention', 'onMentionFile'],
    ['command', 'onStartCommand'],
  ] as const)('routes the native row "%s" to %s and closes the sheet', async (id, action) => {
    const { props } = await openSheet()
    act(() => chrome.listener?.({ type: 'pickPlusItem', id }))
    expect(props[action]).toHaveBeenCalledTimes(1)
    expect(chrome.listener).toBeNull()
  })

  it('closes without running anything when the sheet is dismissed', async () => {
    const { props } = await openSheet()
    act(() => chrome.listener?.({ type: 'closePlusMenu' }))
    expect(chrome.listener).toBeNull()
    expect(props.onOpenAttachFile).not.toHaveBeenCalled()
  })

  it('uploads what the photo library returns like a paste', async () => {
    const { props } = await openSheet()
    const input = screen.getByLabelText('Photo library', { selector: 'input' })
    const click = vi.spyOn(input, 'click')
    act(() => chrome.listener?.({ type: 'pickPlusItem', id: 'photos' }))
    expect(click).toHaveBeenCalledTimes(1)
    const photo = new File(['x'], 'IMG_1.jpg', { type: 'image/jpeg' })
    fireEvent.change(input, { target: { files: [photo] } })
    expect(props.onFiles).toHaveBeenCalledWith([photo])
  })

  it('keeps the menu where the native chrome is absent', async () => {
    platform.mobile = true
    draw()
    await userEvent.setup().click(screen.getByRole('button', { name: /add to this message/i }))
    expect(names(await screen.findAllByRole('menuitem'))).toEqual([
      'Photo library',
      'Take a photo',
      'Attach files',
      'New drawing',
      'Mention a file',
      'Run a command',
    ])
  })
})
