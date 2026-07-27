import { beforeEach, describe, expect, it } from 'vitest'
import {
  clearInputTape,
  dumpInputTape,
  observeInputEvents,
  recordInputTape,
} from '@/features/terminal/utils/input-tape'

describe('input tape', () => {
  beforeEach(() => {
    clearInputTape()
  })

  it('records writes in order with their origin', () => {
    recordInputTape('write', 'onData', 'a')
    recordInputTape('write', 'modifier-enter-override', '\u001b[13;2u')
    recordInputTape('send', 'chunk', 'a\u001b[13;2u')

    const { entries } = dumpInputTape()
    expect(entries.map((e) => [e.kind, e.label, e.data])).toEqual([
      ['write', 'onData', 'a'],
      ['write', 'modifier-enter-override', '\\e[13;2u'],
      ['send', 'chunk', 'a\\e[13;2u'],
    ])
  })

  // Control bytes are the whole point of the recording — an ESC that reads back
  // as an invisible character makes a stream impossible to diff by eye.
  it('escapes control bytes so escape sequences stay legible', () => {
    recordInputTape('write', 'onData', '\u001b[A\r\n\t\u0003')
    expect(dumpInputTape().entries[0].data).toBe('\\e[A\\r\\n\\t\\x03')
  })

  it('captures the DOM events that produce a keystroke, without consuming them', () => {
    const textarea = document.createElement('textarea')
    document.body.appendChild(textarea)
    const stop = observeInputEvents(textarea)

    const keydown = new KeyboardEvent('keydown', {
      key: 'A',
      code: 'KeyA',
      shiftKey: true,
      bubbles: true,
      cancelable: true,
    })
    textarea.dispatchEvent(keydown)
    textarea.dispatchEvent(
      new InputEvent('beforeinput', { data: 'A', inputType: 'insertText', bubbles: true }),
    )

    const { entries } = dumpInputTape()
    expect(entries.map((e) => e.label)).toEqual(['keydown', 'beforeinput'])
    expect(entries[0].meta?.shift).toBe(true)
    expect(entries[1].meta?.inputType).toBe('insertText')
    // The tape must never swallow or cancel what it observes.
    expect(keydown.defaultPrevented).toBe(false)

    stop()
    textarea.dispatchEvent(new KeyboardEvent('keydown', { key: 'B', bubbles: true }))
    expect(dumpInputTape().entries).toHaveLength(2)
    textarea.remove()
  })

  it('is bounded, keeping the most recent entries and counting the evicted', () => {
    for (let i = 0; i < 20_050; i++) recordInputTape('write', 'onData', String(i))
    const dump = dumpInputTape()
    expect(dump.entries).toHaveLength(20_000)
    expect(dump.dropped).toBe(50)
    expect(dump.entries[dump.entries.length - 1].data).toBe('20049')
  })
})
