import { apiFetch } from '@/lib/api/fetch'
import { themeRegistry } from '@/extensions/themes/theme-registry'
import { readTerminalThemePayload } from '../hooks/use-terminal-theme'

const PATH = '/v0/settings/terminal/theme'

interface HostTheme {
  bg: string
  fg: string
}

const keyOf = (theme: HostTheme) => `${theme.bg.toLowerCase()} ${theme.fg.toLowerCase()}`

// The default colours every PTY spawned afterwards answers OSC 10/11 with. `terminalSetTheme`
// reaches only attached sessions, too late for a CLI that queries once at exec (without DEC 2031).
// Boot reads what the daemon holds (a read apiFetch retries while it starts) and pushes only a
// pair it does not hold; the teardown is for HMR.
export function startHostThemeSync(): () => void {
  let disposed = false
  // What the daemon holds; null when unknown (never read, or a push failed).
  let held: string | null = null
  let read = false

  const push = () => {
    if (disposed || !read) return
    const { background, foreground } = readTerminalThemePayload()
    const theme = { bg: background, fg: foreground }
    const key = keyOf(theme)
    if (key === held) return
    held = key
    apiFetch(PATH, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(theme),
    }).catch(() => {
      // Best-effort: the daemon keeps its previous pair, and the next change sends again.
      if (held === key) held = null
    })
  }

  apiFetch<HostTheme>(PATH)
    .then(
      (theme) => {
        held = keyOf(theme)
      },
      () => {
        held = null
      },
    )
    .finally(() => {
      read = true
      push()
    })
  const unlisten = themeRegistry.onThemeChange(push)

  return () => {
    disposed = true
    unlisten()
  }
}
