export type BuildChannel = 'dev' | 'nightly' | 'beta' | 'release'

export interface BuildInfo {
  channel: BuildChannel
  version?: string
  timestamp?: string
}

const DEV_SESSION_START = new Date().toISOString()

interface BuildEnv {
  DEV: boolean
  VITE_BUILD_CHANNEL?: string
  VITE_BUILD_VERSION?: string
  VITE_BUILD_TIMESTAMP?: string
}

function resolveBuildInfo(env: BuildEnv): BuildInfo {
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
