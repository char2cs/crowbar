import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Terminal } from '@xterm/xterm'
import { mountXterm } from '@/features/terminal/lib/mount-xterm'
import { clearInputTape, dumpInputTape } from '@/features/terminal/utils/input-tape'

// The mount's one dispose owns everything it hung on the terminal's DOM: once it
// has run, no listener it added still reacts, even on a detached element.
function mount() {
  const container = document.createElement('div')
  document.body.appendChild(container)
  const write = vi.fn()
  const mounted = mountXterm(
    container,
    {},
    { write, links: { getRoot: () => '/repo', openFile: vi.fn(), openUrl: vi.fn() } },
  )
  return { container, write, mounted }
}

const writeTo = (terminal: Terminal, data: string) =>
  new Promise<void>((resolve) => terminal.write(data, resolve))

/** Fires the copy a Cmd+C sends to the focused terminal; returns what landed on the clipboard. */
function copyFrom(textarea: HTMLTextAreaElement): string | undefined {
  const setData = vi.fn()
  const event = new Event('copy', { bubbles: true, cancelable: true })
  Object.defineProperty(event, 'clipboardData', { value: { setData } })
  textarea.dispatchEvent(event)
  return setData.mock.calls.find(([type]) => type === 'text/plain')?.[1]
}

function replaceText(textarea: HTMLTextAreaElement, data: string) {
  textarea.dispatchEvent(
    new InputEvent('beforeinput', { inputType: 'insertReplacementText', data, cancelable: true }),
  )
}

afterEach(() => {
  document.body.innerHTML = ''
  clearInputTape()
})

describe('mountXterm', () => {
  it('routes a replacement-text input through write while mounted', () => {
    const { mounted, write } = mount()
    const textarea = mounted.terminal.textarea
    if (!textarea) throw new Error('xterm opened without its textarea')

    replaceText(textarea, 'fixed')

    expect(write).toHaveBeenCalledExactlyOnceWith('fixed', 'beforeinput:insertReplacementText')
    mounted.dispose()
  })

  it('copy keeps the newline after a row the program broke at the last column', async () => {
    const { mounted } = mount()
    const { terminal } = mounted
    if (!terminal.textarea) throw new Error('xterm opened without its textarea')
    await writeTo(terminal, 'x'.repeat(terminal.cols) + '\r\nyyyy')
    terminal.select(0, 0, terminal.cols + 4)

    expect(copyFrom(terminal.textarea)).toBe('x'.repeat(terminal.cols) + '\nyyyy')
    mounted.dispose()
  })

  it('copy joins a row the pane width soft-wrapped', async () => {
    const { mounted } = mount()
    const { terminal } = mounted
    if (!terminal.textarea) throw new Error('xterm opened without its textarea')
    await writeTo(terminal, 'x'.repeat(terminal.cols) + 'yyyy')
    terminal.select(0, 0, terminal.cols + 4)

    expect(copyFrom(terminal.textarea)).toBe('x'.repeat(terminal.cols) + 'yyyy')
    mounted.dispose()
  })

  it('dispose releases every textarea listener it added', () => {
    const { mounted, write } = mount()
    const textarea = mounted.terminal.textarea
    if (!textarea) throw new Error('xterm opened without its textarea')

    mounted.dispose()
    clearInputTape()
    replaceText(textarea, 'late')
    textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'a' }))

    expect(write).not.toHaveBeenCalled()
    expect(dumpInputTape().entries).toEqual([])
  })
})
