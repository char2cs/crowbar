import { shellChannel } from './shell-channel'

export interface ShellConfig {
  api: string
  host: string
  ws?: { url: string; token: string } | null
}

interface ConfigWindow {
  __CROWBAR__?: Partial<ShellConfig>
}

export function shellConfig(): ShellConfig | null {
  if (!shellChannel()) return null
  const config = (window as unknown as ConfigWindow).__CROWBAR__
  if (!config?.api || !config.host) return null
  return { api: config.api, host: config.host, ws: config.ws ?? null }
}

export function hostApi(config: ShellConfig, hostId: string): string {
  const scheme = config.api.slice(0, config.api.indexOf('://'))
  return `${scheme}://${hostId}`
}
