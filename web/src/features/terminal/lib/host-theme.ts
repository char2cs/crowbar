import { apiFetch } from '@/lib/api'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { readTerminalThemePayload } from '../hooks/use-terminal-theme'

/**
 * Tell the daemon what the host terminal's default colours are, so every PTY it spawns
 * AFTERWARDS is born answering an OSC 10/11 query with them.
 *
 * This is the half of theme propagation `terminalSetTheme` structurally cannot do. That one
 * travels down a session's own socket, so it can only ever reach a session that already
 * exists — and a vendor CLI is exec'd by the daemon at session-creation time, before any
 * client can attach. A CLI that reads the background once at startup has therefore already
 * asked, and been told x/vt's hardcoded black, by the time the first `terminalSetTheme`
 * lands. Claude Code hid that for a long time because it also subscribes to DEC 2031 and
 * re-queries when the late push notifies it; codex 0.146.0 has no 2031 support at all, so
 * the answer it got at birth was the only one it would ever get, and a light-mode Crowbar
 * came up with a dark Codex.
 *
 * Best-effort by design — a failed push leaves the daemon on its previous value, which is
 * exactly today's behaviour, never a wrong one.
 */
export async function pushHostTerminalTheme(): Promise<boolean> {
  const { background, foreground } = readTerminalThemePayload()
  try {
    await apiFetch('/v0/settings/terminal/theme', {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ bg: background, fg: foreground }),
    })
    return true
  } catch {
    return false
  }
}

/**
 * Boot retry schedule, in ms. The push is the one daemon call that must land BEFORE the
 * user does anything, and it is a PUT — so `apiFetch`'s built-in retry (idempotent reads
 * only) does not cover it, and a daemon still coming up at boot would otherwise leave the
 * host theme unknown until the user happened to switch themes or open a terminal.
 *
 * Bounded and abandoned on the first success: this is a startup handshake, not a poll.
 */
const BOOT_RETRY_DELAYS_MS = [250, 500, 1000, 2000, 4000]

/**
 * Push the host theme now, and again on every theme change, for the lifetime of the app.
 * Returns a teardown for HMR disposal.
 */
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
