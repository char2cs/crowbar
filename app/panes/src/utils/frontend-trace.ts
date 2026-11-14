type TraceLevel = 'debug' | 'info' | 'warn' | 'error'

function shortPath(value: string) {
  const normalized = value.replace(/[\\/]+$/, '')
  const parts = normalized.split(/[\\/]/)
  return parts[parts.length - 1] || value
}

function sanitizePayload(payload?: Record<string, unknown>) {
  if (!payload) return null

  return Object.fromEntries(
    Object.entries(payload).map(([key, value]) => {
      if (typeof value === 'string' && /path/i.test(key)) {
        return [key, shortPath(value)]
      }
      return [key, value]
    }),
  )
}

export function frontendTrace(
  _level: TraceLevel,
  _scope: string,
  _message: string,
  payload?: Record<string, unknown>,
) {
  // No-op: Tauri invoke not available in web frontend
  void sanitizePayload(payload)
}
