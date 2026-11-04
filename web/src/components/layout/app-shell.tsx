import { lazy, Suspense } from 'react'
import { IDEShell } from './ide-shell'
import { IS_MOBILE } from '@/utils/platform'

// The phone shell and its hooks stay out of the desktop boot bundle.
const MobileShell = lazy(() =>
  import('./mobile/mobile-shell').then((m) => ({ default: m.MobileShell })),
)

/** The routed app's frame: the desktop IDE shell, or the phone's single-pane shell. */
export function AppShell() {
  if (!IS_MOBILE) return <IDEShell />
  return (
    <Suspense fallback={null}>
      <MobileShell />
    </Suspense>
  )
}
