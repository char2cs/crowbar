import { apiFetch } from '@/lib/api/fetch'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { readTerminalThemePayload } from '../hooks/use-terminal-theme'

// The default colours every PTY spawned afterwards answers OSC 10/11 with. `terminalSetTheme`
// reaches only attached sessions, too late for a CLI that queries once at exec (without DEC 2031).
// Best-effort: a failed push leaves the daemon's previous value.
export function pushHostTerminalTheme(): Promise<boolean> {
  const { background, foreground } = readTerminalThemePayload()
  const body = JSON.stringify({ bg: background, fg: foreground })
  // The same pair already sent (or on its way) is not sent again: boot pushes
  // once and the initial theme application fires a change for the same value.
  if (lastPush && lastPush.body === body) return lastPush.result
  const push = {
    body,
    result: apiFetch('/v0/settings/terminal/theme', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body,
    }).then(
      () => true,
      () => {
        // A failed push is forgotten so the retry really re-sends.
        if (lastPush === push) lastPush = null
        return false
      },
    ),
  }
  lastPush = push
  return push.result
}

let lastPush: { body: string; result: Promise<boolean> } | null = null

/** Test-only: forget what was last pushed. */
export function resetHostThemePushForTests(): void {
  lastPush = null
}

// The push must land before the user acts, and `apiFetch` retries only idempotent reads, so a
// daemon still booting gets a bounded retry, abandoned on the first success (a handshake, no poll).
const BOOT_RETRY_DELAYS_MS = [250, 500, 1000, 2000, 4000]

/** Push now and on every theme change for the app's lifetime; the teardown is for HMR. */
export function startHostThemeSync(): () => void {
  let disposed = false
  let retryTimer: ReturnType<typeof setTimeout> | null = null

  const clearRetry = () => {
    if (retryTimer !== null) {
      clearTimeout(retryTimer)
      retryTimer = null
    }
  }

  // A theme change supersedes any in-flight boot retry: it pushes the newer value itself,
  // so letting the old schedule run on would only re-send a stale colour pair.
  const pushNow = () => {
    clearRetry()
    void pushHostTerminalTheme()
  }

  const pushWithBootRetry = (attempt: number) => {
    void pushHostTerminalTheme().then((ok) => {
      if (ok || disposed || attempt >= BOOT_RETRY_DELAYS_MS.length) return
      retryTimer = setTimeout(() => {
        retryTimer = null
        pushWithBootRetry(attempt + 1)
      }, BOOT_RETRY_DELAYS_MS[attempt])
    })
  }

  pushWithBootRetry(0)
  const unlisten = themeRegistry.onThemeChange(pushNow)

  return () => {
    disposed = true
    clearRetry()
    unlisten()
  }
}
