import type { ReactNode } from 'react'
import { useProjectHost } from '@/features/hosts/hooks/use-host-status'

// Greyed by re-pointing the colours rows draw in, not by opacity: muted text keeps WCAG AA on the
// sidebar glass, where any opacity low enough to read as greyed does not. The showing Recents
// row's inverted ground greys with it, its change counts drawn in that ground's own ink.
const GREYED_WHILE_UNREACHABLE =
  'data-host-unreachable:[--foreground:var(--muted-foreground)] ' +
  'data-host-unreachable:[--background-inverse:var(--muted-foreground)] ' +
  'data-host-unreachable:[--sidebar-count-added:var(--foreground-inverse)] ' +
  'data-host-unreachable:[--sidebar-count-deleted:var(--foreground-inverse)]'

/** Everything a project lists in the sidebar, greyed out while its remote host can't be reached.
 *  Its rows keep their clicks: what they open says why it can't be reached. */
export function SpaceHostScope({ hostId, children }: { hostId: string; children: ReactNode }) {
  const { unreachable } = useProjectHost(hostId)
  return (
    <div
      data-testid="space-host-scope"
      data-host-unreachable={unreachable || undefined}
      className={GREYED_WHILE_UNREACHABLE}
    >
      {children}
    </div>
  )
}
