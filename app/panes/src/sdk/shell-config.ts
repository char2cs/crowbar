import { shellChannel } from './shell-channel'

/** What the shell tells a page about where its daemon is reachable. */
export interface ShellConfig {
  /** Base of daemon HTTP calls, `crowbar://<host>`. */
  api: string
  host: string
  /** Loopback relay for WebSockets; absent when the shell issued no endpoint. */
  ws?: { url: string; token: string } | null
}

interface ConfigWindow {
  __CROWBAR__?: Partial<ShellConfig>
}

/** The shell's endpoints, or `null` when this page was not loaded by the shell. */
export function shellConfig(): ShellConfig | null {
  if (!shellChannel()) return null
  const config = (window as unknown as ConfigWindow).__CROWBAR__
  if (!config?.api || !config.host) return null
  return { api: config.api, host: config.host, ws: config.ws ?? null }
}

/** The base of `hostId`'s daemon calls: the page's own base, under that host's authority. */
export function hostApi(config: ShellConfig, hostId: string): string {
  const scheme = config.api.slice(0, config.api.indexOf('://'))
  return `${scheme}://${hostId}`
}
