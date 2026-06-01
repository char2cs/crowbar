import type { FaultKey } from '@/lib/store/chaos'

export function shouldFault(request: Request, key: FaultKey): boolean {
  const header = request.headers.get('X-Crowbar-Fault')
  if (!header) return false
  try {
    const faults = JSON.parse(header) as Record<string, number>
    return Math.random() * 100 < (faults[key] ?? 0)
  } catch {
    return false
  }
}
