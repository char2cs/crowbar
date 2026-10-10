import { followStreamFailures } from '@/features/hosts/lib/host-failures'
import { startHostProber } from '@/features/hosts/lib/host-prober'

/** Keeps every paired host's status for the app's life: its probes, and between two probes
 *  whatever its streams are refused with. */
export function startHostStatus(): () => void {
  const stopProber = startHostProber()
  const stopFailures = followStreamFailures()
  return () => {
    stopProber()
    stopFailures()
  }
}
