import { beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
const themeListeners = new Set<() => void>()

vi.mock('@/lib/api/fetch', () => ({
  apiFetch: (path: string, init?: RequestInit) => apiFetch(path, init),
}))

vi.mock('@/extensions/themes/theme-registry', () => ({
  themeRegistry: {
    onThemeChange: (fn: () => void) => {
      themeListeners.add(fn)
      return () => themeListeners.delete(fn)
    },
  },
}))

vi.mock('@/features/terminal/hooks/use-terminal-theme', () => ({
  readTerminalThemePayload: () => readPayload(),
}))

let readPayload: () => { background: string; foreground: string; dark: boolean }

function emitThemeChange() {
  for (const fn of themeListeners) fn()
}

const PATH = '/v0/settings/terminal/theme'
let held: { bg: string; fg: string }

/** Every PUT the module made, as parsed bodies. */
function puts(): Record<string, string>[] {
  return apiFetch.mock.calls
    .filter(([, init]) => (init as RequestInit | undefined)?.method === 'PUT')
    .map(([, init]) => JSON.parse((init as RequestInit).body as string))
}

const { startHostThemeSync } = await import('@/features/terminal/lib/host-theme')

beforeEach(() => {
  apiFetch.mockReset()
  held = { bg: '', fg: '' }
  apiFetch.mockImplementation(async (path: string, init?: RequestInit) => {
    expect(path).toBe(PATH)
    if (init?.method === 'PUT') held = JSON.parse(init.body as string)
    return init?.method === 'PUT' ? undefined : held
  })
  themeListeners.clear()
  readPayload = () => ({ background: '#faf9f5', foreground: '#141414', dark: false })
})

const settle = () => new Promise((resolve) => setTimeout(resolve, 0))

describe('startHostThemeSync', () => {
  it('pushes at boot the colours a fresh daemon does not hold yet', async () => {
    const stop = startHostThemeSync()
    await vi.waitFor(() => expect(puts()).toEqual([{ bg: '#faf9f5', fg: '#141414' }]))
    stop()
  })

  // A reload against a daemon that kept the pair sends nothing at all.
  it('sends nothing at boot when the daemon already holds these colours', async () => {
    held = { bg: '#faf9f5', fg: '#141414' }
    const stop = startHostThemeSync()
    await vi.waitFor(() => expect(apiFetch).toHaveBeenCalledTimes(1))
    await settle()
    expect(puts()).toEqual([])
    stop()
  })

  it('pushes a theme change, once, and not the unchanged pair the theme re-applies', async () => {
    const stop = startHostThemeSync()
    await vi.waitFor(() => expect(puts()).toHaveLength(1))

    readPayload = () => ({ background: '#141413', foreground: '#f5f5f5', dark: true })
    emitThemeChange()
    emitThemeChange()
    await settle()

    expect(puts()).toEqual([
      { bg: '#faf9f5', fg: '#141414' },
      { bg: '#141413', fg: '#f5f5f5' },
    ])
    stop()
  })

  it('sends a pair again after its push failed', async () => {
    held = { bg: '#000000', fg: '#ffffff' }
    let failNext = true
    apiFetch.mockImplementation(async (_path: string, init?: RequestInit) => {
      if (init?.method !== 'PUT') return held
      if (failNext) {
        failNext = false
        throw new Error('ECONNREFUSED')
      }
      return undefined
    })
    const stop = startHostThemeSync()
    await vi.waitFor(() => expect(puts()).toHaveLength(1))

    emitThemeChange()
    await vi.waitFor(() => expect(puts()).toHaveLength(2))
    stop()
  })

  it('stops pushing once torn down (HMR dispose)', async () => {
    const stop = startHostThemeSync()
    await vi.waitFor(() => expect(puts()).toHaveLength(1))
    stop()

    readPayload = () => ({ background: '#141413', foreground: '#f5f5f5', dark: true })
    emitThemeChange()
    await settle()
    expect(puts()).toHaveLength(1)
  })
})
