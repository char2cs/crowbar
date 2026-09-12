export type BuildChannel = 'dev' | 'nightly' | 'beta' | 'release'

export interface BuildInfo {
  channel: BuildChannel
  version?: string
  timestamp?: string
}

// `make dev-desktop` and `bun run dev` both serve through Vite's dev server,
// so `import.meta.env.DEV` alone identifies a dev build — no CI stamp needed.
// Captured once per module load as a stand-in "compiled at" timestamp for the
// dev badge, since an unbundled dev server has no real build step to stamp.
const DEV_SESSION_START = new Date().toISOString()

interface BuildEnv {
  DEV: boolean
  VITE_BUILD_CHANNEL?: string
  VITE_BUILD_VERSION?: string
  VITE_BUILD_TIMESTAMP?: string
}

/**
 * Release CI (nightly.yml, prerelease.yml, stable-release.yml) stamps
 * VITE_BUILD_CHANNEL/VERSION/TIMESTAMP on the frontend build step before
 * `bun run build`. A production bundle built outside that pipeline (a local
 * `bun run build`) carries none of them and falls back to the quiet release
 * badge rather than guessing a channel. Split from `getBuildInfo()` so the
 * branching is testable without stubbing Vite's own `import.meta.env`.
 */
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
