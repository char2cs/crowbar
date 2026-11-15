import { afterEach, describe, expect, it } from 'vitest'
import { bufferSession } from '@/entries/buffer-session'

afterEach(() => bufferSession.setState({ chatId: null, saved: null, active: false }))

describe('bufferSession', () => {
  it('starts with no chat, nothing saved and the pane inactive', () => {
    expect(bufferSession.getState()).toEqual({ chatId: null, saved: null, active: false })
  })

  it('holds what the shell last said about the pane', () => {
    bufferSession.setState({ chatId: 'c1', saved: { path: 'a.ts' }, active: true })
    expect(bufferSession.getState().chatId).toBe('c1')
  })
})
