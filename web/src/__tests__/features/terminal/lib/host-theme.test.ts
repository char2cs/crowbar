import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

const apiFetch = vi.fn()
const themeListeners = new Set<() => void>()

vi.mock('@/lib/api', () => ({
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

/** The parsed body of the Nth PUT the module made. */
function bodyOf(call: number): Record<string, string> {
  return JSON.parse((apiFetch.mock.calls[call][1] as RequestInit).body as string)
}

const { pushHostTerminalTheme, startHostThemeSync } =
  await import('@/features/terminal/lib/host-theme')

beforeEach(() => {
  vi.useFakeTimers()
  apiFetch.mockReset()
  apiFetch.mockResolvedValue(undefined)
  themeListeners.clear()
  readPayload = () => ({ background: '#faf9f5', foreground: '#141414', dark: false })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('pushHostTerminalTheme', () => {
  it('PUTs the resolved host colours to the daemon settings endpoint', async () => {
    await expect(pushHostTerminalTheme()).resolves.toBe(true)

    expect(apiFetch).toHaveBeenCalledTimes(1)
    expect(apiFetch.mock.calls[0][0]).toBe('/v0/settings/terminal/theme')
    expect((apiFetch.mock.calls[0][1] as RequestInit).method).toBe('PUT')
    expect(bodyOf(0)).toEqual({ bg: '#faf9f5', fg: '#141414' })
  })

  it('reports failure instead of throwing, so a boot before the daemon is up is survivable', async () => {
    apiFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    await expect(pushHostTerminalTheme()).resolves.toBe(false)
  })
})

describe('startHostThemeSync', () => {
  it('pushes immediately at boot — the daemon must know before any CLI is spawned', async () => {
    const stop = startHostThemeSync()
    await vi.advanceTimersByTimeAsync(0)

    expect(apiFetch).toHaveBeenCalledTimes(1)
    expect(bodyOf(0)).toEqual({ bg: '#faf9f5', fg: '#141414' })
    stop()
  })

  it('re-pushes on every theme change so the NEXT chat inherits the new polarity', async () => {
    const stop = startHostThemeSync()
    await vi.advanceTimersByTimeAsync(0)

    readPayload = () => ({ background: '#141413', foreground: '#f5f5f5', dark: true })
    emitThemeChange()
    await vi.advanceTimersByTimeAsync(0)

    expect(apiFetch).toHaveBeenCalledTimes(2)
    expect(bodyOf(1)).toEqual({ bg: '#141413', fg: '#f5f5f5' })
    stop()
  })

  it('retries a failed boot push until it lands — a daemon still starting must not lose the theme', async () => {
    apiFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    apiFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))
    apiFetch.mockResolvedValue(undefined)

    const stop = startHostThemeSync()
    await vi.advanceTimersByTimeAsync(0)
    expect(apiFetch).toHaveBeenCalledTimes(1)

    await vi.advanceTimersByTimeAsync(250)
    expect(apiFetch).toHaveBeenCalledTimes(2)

    await vi.advanceTimersByTimeAsync(500)
    expect(apiFetch).toHaveBeenCalledTimes(3)

    // Landed — the schedule stops rather than continuing to poll.
    await vi.advanceTimersByTimeAsync(60_000)
    expect(apiFetch).toHaveBeenCalledTimes(3)
    stop()
  })

  it('lets a theme change supersede a pending boot retry rather than re-sending a stale colour', async () => {
    apiFetch.mockRejectedValueOnce(new Error('ECONNREFUSED'))

    const stop = startHostThemeSync()
    await vi.advanceTimersByTimeAsync(0)
    expect(apiFetch).toHaveBeenCalledTimes(1)

    readPayload = () => ({ background: '#141413', foreground: '#f5f5f5', dark: true })
    emitThemeChange()
    await vi.advanceTimersByTimeAsync(0)
    expect(apiFetch).toHaveBeenCalledTimes(2)
    expect(bodyOf(1)).toEqual({ bg: '#141413', fg: '#f5f5f5' })

    // The abandoned retry must not fire behind the newer push.
    await vi.advanceTimersByTimeAsync(60_000)
    expect(apiFetch).toHaveBeenCalledTimes(2)
    stop()
  })

  it('stops pushing once torn down (HMR dispose)', async () => {
    const stop = startHostThemeSync()
    await vi.advanceTimersByTimeAsync(0)
    stop()

    emitThemeChange()
    await vi.advanceTimersByTimeAsync(60_000)
    expect(apiFetch).toHaveBeenCalledTimes(1)
  })
})
