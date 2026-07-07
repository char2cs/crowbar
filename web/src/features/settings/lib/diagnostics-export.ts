import { isTauri } from '@/lib/crowbar-bridge'

/**
 * Runs the desktop `diagnostics_export` command: bundles the daemon log
 * (panic traces, watchdog goroutine dumps), the app log, fresh
 * goroutine/heap dumps from the live daemon, and version metadata into a
 * zip in ~/Downloads. Resolves to the bundle's absolute path.
 */
export async function exportDiagnostics(): Promise<string> {
  if (!isTauri()) {
    throw new Error('Diagnostics export requires the desktop app')
  }
  // Use the global injected by Tauri before any JS runs — no npm import
  // needed (same pattern as tauriInvoke in crowbar-bridge).
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const path = await (window as any).__TAURI_INTERNALS__.invoke('diagnostics_export')
  return path as string
}
