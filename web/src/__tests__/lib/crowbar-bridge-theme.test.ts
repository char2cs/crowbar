import { describe, it, expect, vi, beforeEach } from 'vitest'
// Browser path is the jsdom default (no __TAURI_INTERNALS__); no Tauri mock needed.
import { terminalAttach, terminalSetTheme } from '@/lib/crowbar-bridge'

// A WebSocket that does NOT auto-open, so a test can push a frame during the connecting
// window and then fire onopen manually.
class ManualWebSocket {
  static instances: ManualWebSocket[] = []
  onopen: (() => void) | null = null
  onmessage: ((e: { data: string }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  constructor(public url: string) {
    ManualWebSocket.instances.push(this)
  }
  send = vi.fn()
  close = vi.fn()
}

beforeEach(() => {
  ManualWebSocket.instances = []
  vi.stubGlobal('WebSocket', ManualWebSocket as unknown as typeof WebSocket)
})

describe('terminalSetTheme', () => {
  it('queues the theme frame while the socket is connecting and flushes it on open', async () => {
    await terminalAttach('conn-theme', '/base')
    const ws = ManualWebSocket.instances[0]

    // Socket has not opened yet — mirrors the on-attach push racing the WS handshake.
    await terminalSetTheme('conn-theme', {
      background: '#101014',
      foreground: '#e8e8e8',
      dark: true,
    })
    expect(ws.send).not.toHaveBeenCalled() // held, not dropped

    ws.onopen?.() // handshake completes
    expect(ws.send).toHaveBeenCalledTimes(1)
    expect(JSON.parse(ws.send.mock.calls[0][0] as string)).toEqual({
      type: 'theme',
      bg: '#101014',
      fg: '#e8e8e8',
      dark: true,
    })
  })

  it('coalesces to the LAST theme pushed before open', async () => {
    await terminalAttach('conn-theme-coalesce', '/base')
    const ws = ManualWebSocket.instances[0]

    await terminalSetTheme('conn-theme-coalesce', {
      background: '#000',
      foreground: '#fff',
      dark: true,
    })
    await terminalSetTheme('conn-theme-coalesce', {
      background: '#fff',
      foreground: '#000',
      dark: false,
    })

    ws.onopen?.()
    expect(ws.send).toHaveBeenCalledTimes(1)
    expect(JSON.parse(ws.send.mock.calls[0][0] as string)).toMatchObject({ dark: false })
  })

  it('sends immediately when the socket is already open', async () => {
    await terminalAttach('conn-theme-open', '/base')
    const ws = ManualWebSocket.instances[0]
    ws.onopen?.() // open now (no queued theme yet)
    ws.send.mockClear()

    await terminalSetTheme('conn-theme-open', {
      background: '#ffffff',
      foreground: '#111111',
      dark: false,
    })
    expect(ws.send).toHaveBeenCalledTimes(1)
    expect(JSON.parse(ws.send.mock.calls[0][0] as string)).toEqual({
      type: 'theme',
      bg: '#ffffff',
      fg: '#111111',
      dark: false,
    })
  })
})
