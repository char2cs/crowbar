import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { restartDaemon, revealDaemonLog } from '@/lib/crowbar-bridge'
import { useDaemonCrashStore } from '@/features/window/stores/daemon-crash-store'
import { toast } from '@/features/window/stores/toast-store'

const errorText = (err: unknown) => (err instanceof Error ? err.message : String(err))

// The crash loop's only way out: the supervisor stopped respawning the backend, so nothing
// reconnects until the user restarts it. Drawn over the app rather than in place of it, so the
// open tabs and their unsaved edits stay mounted.
export function DaemonCrashPanel() {
  const report = useDaemonCrashStore((state) => state.report)
  const [restarting, setRestarting] = useState(false)
  const [failure, setFailure] = useState<string | null>(null)
  if (report === null) return null

  const restart = async () => {
    setRestarting(true)
    setFailure(null)
    try {
      await restartDaemon()
    } catch (err) {
      setFailure(errorText(err))
    } finally {
      setRestarting(false)
    }
  }
  const openLog = () => {
    revealDaemonLog().catch((err: unknown) => toast.error('Could not open the log', errorText(err)))
  }
  const copyReport = () => {
    navigator.clipboard.writeText(report).then(
      () => toast.success('Diagnostics copied'),
      (err: unknown) => toast.error('Could not copy diagnostics', errorText(err)),
    )
  }

  return (
    <div
      role="alert"
      aria-labelledby="daemon-crash-title"
      className="fixed bottom-4 right-4 z-[100] flex w-[min(28rem,calc(100vw-2rem))] flex-col gap-3 rounded-lg border border-destructive/30 bg-popover p-4 text-popover-foreground shadow-lg"
    >
      <div className="space-y-1">
        <h2 id="daemon-crash-title" className="text-sm font-medium text-destructive">
          Crowbar's backend keeps crashing
        </h2>
        <p className="text-xs text-muted-foreground">
          Crowbar stopped restarting it on its own. Your open tabs and unsaved edits are kept;
          nothing reconnects until the backend runs again.
        </p>
      </div>
      <pre className="max-h-40 overflow-auto rounded bg-muted/50 p-2 font-mono text-[11px] whitespace-pre-wrap wrap-anywhere text-muted-foreground">
        {report}
      </pre>
      {failure && <p className="text-xs text-destructive">Restart failed: {failure}</p>}
      <div className="flex flex-wrap justify-end gap-2">
        <Button variant="ghost" size="sm" onClick={copyReport}>
          Copy diagnostics
        </Button>
        <Button variant="ghost" size="sm" onClick={openLog}>
          Open log
        </Button>
        <Button size="sm" loading={restarting} onClick={() => void restart()}>
          Restart backend
        </Button>
      </div>
    </div>
  )
}
