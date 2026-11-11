export type BuildChannel = 'dev' | 'nightly' | 'beta' | 'release'

export interface BuildInfo {
  channel: BuildChannel
  version?: string
  timestamp?: string
}

// Both dev commands serve through Vite, so `import.meta.env.DEV` alone identifies a dev build.
// The module-load time stands in as the dev badge's "compiled at", since dev has no build step.
const DEV_SESSION_START = new Date().toISOString()

interface BuildEnv {
  DEV: boolean
  VITE_BUILD_CHANNEL?: string
  VITE_BUILD_VERSION?: string
  VITE_BUILD_TIMESTAMP?: string
}

// Release CI stamps VITE_BUILD_CHANNEL/VERSION/TIMESTAMP before `bun run build`; a bundle built
// outside it falls back to the quiet release badge rather than guess a channel. Split from
// getBuildInfo() so the branching is testable without stubbing `import.meta.env`.
export function resolveBuildInfo(env: BuildEnv): BuildInfo {
  if (env.DEV) return { channel: 'dev', timestamp: DEV_SESSION_START }

  const version = env.VITE_BUILD_VERSION || __APP_VERSION__
  if (env.VITE_BUILD_CHANNEL === 'nightly') {
    return { channel: 'nightly', timestamp: env.VITE_BUILD_TIMESTAMP }
  }
  if (env.VITE_BUILD_CHANNEL === 'beta') {
    return { channel: 'beta', version, timestamp: env.VITE_BUILD_TIMESTAMP }
  }
  return { channel: 'release', version }
}

export function getBuildInfo(): BuildInfo {
  return resolveBuildInfo(import.meta.env)
}
