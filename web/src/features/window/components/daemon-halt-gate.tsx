import type { ReactNode } from 'react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useDaemonHaltStore } from '@/features/window/stores/daemon-halt-store'

/**
 * Renders `children` unless the daemon halted, in which case the halt replaces
 * them. It is the one place a halt is told to the user: a toast has no viewport
 * outside the IDE shell, so a halt on a first run would never be seen.
 */
export function DaemonHaltGate({ children }: { children: ReactNode }) {
  const message = useDaemonHaltStore((state) => state.message)
  if (message === null) return <>{children}</>
  return (
    <div role="alert" className="relative flex min-h-screen items-center justify-center p-8">
      <div data-tauri-drag-region className="absolute inset-x-0 top-0 z-20 h-8" />
      <Card className="w-full max-w-md border-destructive/20 bg-destructive/10">
        <CardHeader className="pb-2">
          <CardTitle className="text-sm text-destructive">Crowbar could not start</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <p className="font-mono text-[11px] text-muted-foreground">{message}</p>
          <p className="text-xs text-muted-foreground">
            Quit Crowbar, fix the cause above, and open it again.
          </p>
        </CardContent>
      </Card>
    </div>
  )
}
