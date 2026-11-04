import { describe, expect, it, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { TerminalKeyBar } from '@/features/terminal/components/terminal-key-bar'

function mount() {
  const onSend = vi.fn()
  render(<TerminalKeyBar onSend={onSend} />)
  return onSend
}

const key = (name: RegExp) => fireEvent.click(screen.getByRole('button', { name }))

describe('TerminalKeyBar', () => {
  it.each([
    [/^escape$/i, '\x1b'],
    [/^tab$/i, '\t'],
    [/^arrow up$/i, '\x1b[A'],
    [/^arrow down$/i, '\x1b[B'],
    [/^arrow right$/i, '\x1b[C'],
    [/^arrow left$/i, '\x1b[D'],
  ])('%s sends its key', (name, bytes) => {
    const onSend = mount()
    key(name)
    expect(onSend).toHaveBeenCalledExactlyOnceWith(bytes)
  })

  it('sends a typed line followed by Return, and clears the field', () => {
    const onSend = mount()
    const field = screen.getByRole('textbox', { name: /terminal input/i })
    fireEvent.change(field, { target: { value: 'ls -la' } })
    fireEvent.submit(field)
    expect(onSend).toHaveBeenCalledExactlyOnceWith('ls -la\r')
    expect(field).toHaveValue('')
  })

  it('sends a bare Return for an empty line', () => {
    const onSend = mount()
    fireEvent.submit(screen.getByRole('textbox', { name: /terminal input/i }))
    expect(onSend).toHaveBeenCalledExactlyOnceWith('\r')
  })

  // Ctrl is latched for one key: Ctrl, then "c", is ^C; the next key is plain again.
  it('turns the next character into its Ctrl chord, once', () => {
    const onSend = mount()
    const ctrl = screen.getByRole('button', { name: /^control$/i })
    fireEvent.click(ctrl)
    expect(ctrl).toHaveAttribute('aria-pressed', 'true')
    const field = screen.getByRole('textbox', { name: /terminal input/i })
    fireEvent.change(field, { target: { value: 'c' } })
    expect(onSend).toHaveBeenCalledExactlyOnceWith('\x03')
    expect(field).toHaveValue('')
    expect(ctrl).toHaveAttribute('aria-pressed', 'false')
  })
})
