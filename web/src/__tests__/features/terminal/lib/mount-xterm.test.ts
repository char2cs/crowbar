import { afterEach, describe, expect, it, vi } from 'vitest'
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
    { write, fileLinks: { getRoot: () => '/repo', openFile: vi.fn() } },
  )
  return { container, write, mounted }
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
